"""
Integrated Transcription Service

Orchestrates the complete transcription pipeline with quality assessment,
routing decisions, confidence scoring, and fallback handling.
"""
from pathlib import Path
from typing import Dict, Any, Optional, Tuple
from datetime import datetime
import asyncio
import uuid

from app.transcription.whisper_local import whisper_processor
from app.transcription.cloud_stt import google_cloud_stt
from app.transcription.quality import audio_quality_analyzer
from app.config import settings


class TranscriptionService:
    """Orchestrates transcription pipeline with quality assessment and routing"""

    def __init__(self):
        self.quality_analyzer = audio_quality_analyzer
        self.local_processor = whisper_processor
        self.cloud_processor = google_cloud_stt

        # Quality thresholds
        self.min_confidence_threshold = settings.STT_CONFIDENCE_THRESHOLD
        self.retry_threshold = 0.6  # Retry with cloud if local confidence < 60%

    async def transcribe_meeting(
        self,
        audio_path: Path,
        meeting_id: str,
        language: str = "en",
        force_local: bool = False,
        force_cloud: bool = False
    ) -> Dict[str, Any]:
        """Main transcription entry point with intelligent routing"""

        start_time = datetime.utcnow()
        transcription_id = str(uuid.uuid4())

        print(f"🎤 Starting transcription for meeting {meeting_id}")
        print(f"📁 Audio file: {audio_path.name} ({audio_path.stat().st_size / 1024 / 1024:.1f} MB)")

        try:
            # Step 1: Audio quality assessment (unless forced)
            if not force_local and not force_cloud:
                quality_analysis = await self.quality_analyzer.analyze_audio_quality(audio_path)
                routing_decision = quality_analysis["routing_decision"]
                print(f"🔍 Quality analysis complete: {routing_decision['recommended_service']}")
            else:
                quality_analysis = {"routing_decision": {"use_local": not force_cloud}}
                routing_decision = quality_analysis["routing_decision"]

            # Step 2: Primary transcription attempt
            primary_result = await self._perform_primary_transcription(
                audio_path,
                routing_decision,
                language,
                force_local,
                force_cloud
            )

            # Step 3: Quality validation and potential fallback
            final_result = await self._validate_and_fallback(
                primary_result,
                audio_path,
                language,
                force_local,
                force_cloud
            )

            # Step 4: Build comprehensive response
            end_time = datetime.utcnow()
            total_duration = (end_time - start_time).total_seconds()

            comprehensive_result = {
                "transcription_id": transcription_id,
                "meeting_id": meeting_id,
                "audio_file": str(audio_path),
                "language": language,
                "total_processing_time": total_duration,
                "quality_analysis": quality_analysis,
                "transcription_result": final_result,
                "pipeline_metadata": {
                    "started_at": start_time.isoformat() + "Z",
                    "completed_at": end_time.isoformat() + "Z",
                    "service_used": final_result.get("transcription_method", "unknown"),
                    "fallback_used": final_result.get("fallback_used", False),
                    "confidence_score": final_result.get("confidence_score", 0.0),
                    "quality_score": quality_analysis.get("overall_quality_score", 0.0)
                }
            }

            print(f"✅ Transcription complete in {total_duration:.1f}s")
            print(f"🎯 Final confidence: {final_result.get('confidence_score', 0):.2f}")

            return comprehensive_result

        except Exception as e:
            print(f"❌ Transcription failed: {str(e)}")
            error_result = {
                "transcription_id": transcription_id,
                "meeting_id": meeting_id,
                "error": str(e),
                "status": "failed",
                "attempted_at": start_time.isoformat() + "Z"
            }
            return error_result

    async def _perform_primary_transcription(
        self,
        audio_path: Path,
        routing_decision: Dict[str, Any],
        language: str,
        force_local: bool,
        force_cloud: bool
    ) -> Dict[str, Any]:
        """Perform primary transcription based on routing decision"""

        if force_cloud or (not force_local and not routing_decision.get("use_local", True)):
            # Use cloud transcription
            print("☁️ Using Google Cloud Speech-to-Text")

            if not self.cloud_processor.is_available():
                print("⚠️ Cloud STT not available, falling back to local")
                return await self._transcribe_local(audio_path, language, fallback=True)

            try:
                result = await self.cloud_processor.transcribe_audio(
                    audio_path,
                    language_code=f"{language}-US" if language == "en" else language
                )
                result["fallback_used"] = False
                return result

            except Exception as e:
                print(f"❌ Cloud transcription failed: {str(e)}")
                print("🔄 Falling back to local transcription")
                return await self._transcribe_local(audio_path, language, fallback=True)

        else:
            # Use local transcription
            print("💻 Using local WhisperX")
            return await self._transcribe_local(audio_path, language, fallback=False)

    async def _transcribe_local(
        self,
        audio_path: Path,
        language: str,
        fallback: bool = False
    ) -> Dict[str, Any]:
        """Perform local WhisperX transcription"""

        try:
            # Ensure models are loaded
            if not self.local_processor.whisper_model:
                await self.local_processor.initialize_models(language)

            result = await self.local_processor.transcribe_audio(audio_path, language)
            result["fallback_used"] = fallback
            return result

        except Exception as e:
            if fallback:
                # If this was already a fallback, propagate the error
                raise RuntimeError(f"Local transcription fallback failed: {str(e)}")
            else:
                raise RuntimeError(f"Local transcription failed: {str(e)}")

    async def _validate_and_fallback(
        self,
        result: Dict[str, Any],
        audio_path: Path,
        language: str,
        force_local: bool,
        force_cloud: bool
    ) -> Dict[str, Any]:
        """Validate transcription quality and fallback if needed"""

        # Check if we already had an error
        if "error" in result:
            return result

        # Get confidence score
        confidence = result.get("confidence_score", 0.0)

        # Check if confidence is below retry threshold and we haven't used cloud yet
        should_retry_with_cloud = (
            confidence < self.retry_threshold and
            not force_local and
            not result.get("fallback_used", False) and
            result.get("transcription_method", "").startswith("whisperx") and
            self.cloud_processor.is_available()
        )

        if should_retry_with_cloud:
            print(f"🔄 Low confidence ({confidence:.2f}), retrying with cloud STT")

            try:
                cloud_result = await self.cloud_processor.transcribe_audio(
                    audio_path,
                    language_code=f"{language}-US" if language == "en" else language
                )

                cloud_confidence = cloud_result.get("confidence_score", 0.0)

                # Use cloud result if it has higher confidence
                if cloud_confidence > confidence:
                    print(f"✅ Cloud result better ({cloud_confidence:.2f} vs {confidence:.2f})")
                    cloud_result["fallback_used"] = True
                    cloud_result["original_local_confidence"] = confidence
                    return cloud_result
                else:
                    print(f"📊 Local result still better ({confidence:.2f} vs {cloud_confidence:.2f})")
                    result["cloud_fallback_attempted"] = True
                    result["cloud_confidence"] = cloud_confidence
                    return result

            except Exception as e:
                print(f"⚠️ Cloud fallback failed: {str(e)}")
                result["cloud_fallback_error"] = str(e)
                return result

        return result

    async def get_service_status(self) -> Dict[str, Any]:
        """Get status of all transcription services"""

        # Test local service
        local_status = await self.local_processor.get_model_info()

        # Test cloud service
        cloud_status = await self.cloud_processor.test_cloud_connection()

        return {
            "local_whisperx": {
                "available": local_status["models_loaded"]["whisper"],
                "device": local_status["device"],
                "model_size": local_status["whisper_model_size"],
                "memory_usage": local_status["memory_usage"]
            },
            "cloud_stt": {
                "available": cloud_status["available"],
                "service": "google_cloud_speech",
                "credentials_configured": cloud_status.get("credentials_configured", False)
            },
            "quality_analyzer": {
                "available": True,
                "snr_threshold": self.quality_analyzer.snr_threshold,
                "confidence_threshold": self.min_confidence_threshold
            },
            "pipeline": {
                "retry_threshold": self.retry_threshold,
                "forced_local_mode": settings.USE_LOCAL_STT_ONLY
            }
        }

    async def estimate_processing_time(self, audio_path: Path) -> Dict[str, Any]:
        """Estimate processing time for audio file"""

        try:
            # Get audio duration
            import librosa
            audio_data, sample_rate = librosa.load(str(audio_path), sr=None, duration=1.0)
            estimated_duration = audio_path.stat().st_size / (sample_rate * 2)  # Rough estimate

            # Estimate local processing (2x realtime on Mac M4)
            local_estimate = estimated_duration * 2

            # Estimate cloud processing (3-5x realtime + upload time)
            upload_time = audio_path.stat().st_size / (1024 * 1024)  # 1 MB/s estimate
            cloud_estimate = estimated_duration * 4 + upload_time

            # Quality check time (usually <10 seconds)
            quality_check_time = min(10, estimated_duration * 0.1)

            return {
                "audio_duration_seconds": estimated_duration,
                "quality_check_seconds": quality_check_time,
                "local_estimate_seconds": local_estimate,
                "cloud_estimate_seconds": cloud_estimate,
                "recommended_service": "local" if local_estimate < cloud_estimate else "cloud",
                "total_estimate_seconds": quality_check_time + min(local_estimate, cloud_estimate)
            }

        except Exception as e:
            return {
                "error": f"Time estimation failed: {str(e)}",
                "fallback_estimate_seconds": 300  # 5 minute default
            }

    async def quick_transcription_test(self) -> Dict[str, Any]:
        """Quick test of transcription services"""

        tests = {}

        # Test local service
        try:
            local_test = await self.local_processor.test_transcription()
            tests["local_whisperx"] = local_test
        except Exception as e:
            tests["local_whisperx"] = {"test_status": "failed", "error": str(e)}

        # Test cloud service
        try:
            cloud_test = await self.cloud_processor.test_cloud_connection()
            tests["cloud_stt"] = cloud_test
        except Exception as e:
            tests["cloud_stt"] = {"available": False, "error": str(e)}

        # Test quality analyzer
        try:
            tests["quality_analyzer"] = {"available": True, "status": "ready"}
        except Exception as e:
            tests["quality_analyzer"] = {"available": False, "error": str(e)}

        return {
            "service_tests": tests,
            "overall_status": "ready" if all(
                test.get("available") or test.get("test_status") == "success"
                for test in tests.values()
            ) else "degraded"
        }


# Global transcription service instance
transcription_service = TranscriptionService()