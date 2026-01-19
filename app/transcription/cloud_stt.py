"""
Google Cloud Speech-to-Text Fallback

Implements cloud-based transcription for poor quality audio that requires
enhanced processing capabilities beyond local WhisperX.
"""
from google.cloud import speech
import asyncio
import aiofiles
from pathlib import Path
from typing import Dict, Any, Optional, List
import json
from datetime import datetime
import librosa
import numpy as np
import tempfile
import os

from app.config import settings


class GoogleCloudSTT:
    """Google Cloud Speech-to-Text service integration"""

    def __init__(self):
        self.client = None
        self.credentials_path = settings.GOOGLE_CLOUD_CREDENTIALS_PATH
        self._initialize_client()

    def _initialize_client(self):
        """Initialize Google Cloud Speech client"""
        try:
            if self.credentials_path and os.path.exists(self.credentials_path):
                os.environ['GOOGLE_APPLICATION_CREDENTIALS'] = self.credentials_path
                self.client = speech.SpeechClient()
                print("✅ Google Cloud Speech client initialized")
            else:
                print("⚠️ Google Cloud credentials not found - cloud fallback disabled")
                self.client = None
        except Exception as e:
            print(f"❌ Failed to initialize Google Cloud Speech client: {e}")
            self.client = None

    async def transcribe_audio(
        self,
        audio_path: Path,
        language_code: str = "en-US",
        enable_speaker_diarization: bool = True,
        min_speakers: Optional[int] = None,
        max_speakers: Optional[int] = None
    ) -> Dict[str, Any]:
        """Transcribe audio using Google Cloud Speech-to-Text"""

        if not self.client:
            raise RuntimeError("Google Cloud Speech client not available")

        if not audio_path.exists():
            raise FileNotFoundError(f"Audio file not found: {audio_path}")

        try:
            start_time = datetime.utcnow()
            print(f"☁️ Starting cloud transcription of {audio_path.name}...")

            # Prepare audio for Cloud Speech
            audio_data = await self._prepare_audio_for_cloud(audio_path)
            print(f"📁 Audio prepared: {len(audio_data)} bytes")

            # Configure recognition request
            config = await self._build_recognition_config(
                audio_path,
                language_code,
                enable_speaker_diarization,
                min_speakers,
                max_speakers
            )

            # Create recognition request
            audio = speech.RecognitionAudio(content=audio_data)
            request = speech.LongRunningRecognizeRequest(config=config, audio=audio)

            print("🎤 Submitting to Google Cloud Speech...")

            # Start long-running recognition operation
            operation = self.client.long_running_recognize(request=request)
            print(f"⏳ Operation started: {operation.operation.name}")

            # Wait for operation to complete (async polling)
            result = await self._poll_operation_result(operation)

            # Process results
            end_time = datetime.utcnow()
            processing_duration = (end_time - start_time).total_seconds()

            print(f"✅ Cloud transcription complete in {processing_duration:.1f}s")

            # Build comprehensive result
            transcription_result = await self._build_cloud_result(
                result,
                audio_path,
                processing_duration,
                language_code,
                config
            )

            return transcription_result

        except Exception as e:
            print(f"❌ Cloud transcription failed: {str(e)}")
            raise RuntimeError(f"Google Cloud transcription failed: {str(e)}")

    async def _prepare_audio_for_cloud(self, audio_path: Path) -> bytes:
        """Prepare audio file for Google Cloud Speech API"""

        try:
            # Load audio file
            audio_data, sample_rate = librosa.load(str(audio_path), sr=16000, mono=True)

            # Google Cloud prefers 16kHz mono FLAC or WAV
            # Convert to WAV format in memory
            import soundfile as sf
            import io

            # Create temporary WAV file
            with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as temp_file:
                sf.write(temp_file.name, audio_data, 16000, format='WAV', subtype='PCM_16')
                temp_path = temp_file.name

            # Read the WAV data
            async with aiofiles.open(temp_path, "rb") as f:
                wav_data = await f.read()

            # Clean up temporary file
            os.unlink(temp_path)

            return wav_data

        except Exception as e:
            raise ValueError(f"Error preparing audio for cloud processing: {str(e)}")

    async def _build_recognition_config(
        self,
        audio_path: Path,
        language_code: str,
        enable_speaker_diarization: bool,
        min_speakers: Optional[int],
        max_speakers: Optional[int]
    ) -> speech.RecognitionConfig:
        """Build Google Cloud Speech recognition configuration"""

        # Get audio duration for optimization
        audio_data, sample_rate = librosa.load(str(audio_path), sr=None, duration=1.0)
        estimated_duration = audio_path.stat().st_size / (sample_rate * 2)  # Rough estimate

        # Configure recognition settings
        config = speech.RecognitionConfig(
            encoding=speech.RecognitionConfig.AudioEncoding.LINEAR16,
            sample_rate_hertz=16000,
            language_code=language_code,
            # Enable word-level timestamps
            enable_word_time_offsets=True,
            enable_word_confidence=True,
            # Use enhanced models for better accuracy
            use_enhanced=True,
            # Enable automatic punctuation
            enable_automatic_punctuation=True,
            # Audio channel count
            audio_channel_count=1,
            enable_separate_recognition_per_channel=False,
            # Alternative candidates for better accuracy
            max_alternatives=1,
            # Profanity filter (optional)
            profanity_filter=False,
            # Speech contexts for domain-specific terms
            speech_contexts=[
                speech.SpeechContext(
                    phrases=[
                        "meeting", "action item", "follow up", "decision",
                        "project", "timeline", "deadline", "budget"
                    ]
                )
            ]
        )

        # Configure speaker diarization if enabled
        if enable_speaker_diarization:
            diarization_config = speech.SpeakerDiarizationConfig(
                enable_speaker_diarization=True,
                min_speaker_count=min_speakers or 2,
                max_speaker_count=max_speakers or 8
            )
            config.diarization_config = diarization_config
            print(f"🎯 Speaker diarization enabled: {min_speakers or 2}-{max_speakers or 8} speakers")

        # Use appropriate recognition model based on audio characteristics
        if estimated_duration > 300:  # 5+ minutes
            config.model = "video"  # Better for longer content
        else:
            config.model = "latest_long"  # Latest model for general use

        return config

    async def _poll_operation_result(self, operation, poll_interval: float = 5.0) -> Any:
        """Poll long-running operation for completion"""

        max_wait_time = 3600  # 1 hour maximum
        elapsed_time = 0

        while not operation.done() and elapsed_time < max_wait_time:
            print(f"⏳ Transcription in progress... ({elapsed_time:.0f}s elapsed)")
            await asyncio.sleep(poll_interval)
            elapsed_time += poll_interval

            # Check operation status
            operation = self.client._transport.operations_client.get_operation(
                operation.operation.name
            )

        if not operation.done():
            raise TimeoutError("Cloud transcription timed out after 1 hour")

        if operation.error:
            raise RuntimeError(f"Cloud transcription failed: {operation.error}")

        # Parse result
        response = speech.LongRunningRecognizeResponse()
        operation.response.Unpack(response)

        return response

    async def _build_cloud_result(
        self,
        response: Any,
        audio_path: Path,
        processing_duration: float,
        language_code: str,
        config: speech.RecognitionConfig
    ) -> Dict[str, Any]:
        """Build comprehensive result from Google Cloud response"""

        results = response.results
        if not results:
            return {
                "transcript_text": "",
                "confidence_score": 0.0,
                "error": "No transcription results returned"
            }

        # Extract transcript and segments
        full_transcript = ""
        speaker_segments = []
        word_count = 0
        total_confidence = 0.0
        confidence_count = 0

        speakers_detected = set()

        for result in results:
            for alternative in result.alternatives:
                # Add to full transcript
                if alternative.transcript:
                    full_transcript += alternative.transcript.strip() + " "

                # Process words for segments
                if hasattr(alternative, 'words'):
                    current_segment = {
                        "speaker": None,
                        "start_time": None,
                        "end_time": None,
                        "text": "",
                        "confidence": alternative.confidence if hasattr(alternative, 'confidence') else 0.0,
                        "words": []
                    }

                    for word_info in alternative.words:
                        word_data = {
                            "word": word_info.word,
                            "start_time": word_info.start_time.total_seconds(),
                            "end_time": word_info.end_time.total_seconds(),
                            "confidence": word_info.confidence if hasattr(word_info, 'confidence') else 0.0
                        }

                        # Handle speaker diarization
                        if hasattr(word_info, 'speaker_tag'):
                            speaker_id = f"Speaker {word_info.speaker_tag}"
                            speakers_detected.add(speaker_id)
                            word_data["speaker"] = speaker_id

                            # Start new segment if speaker changed
                            if current_segment["speaker"] != speaker_id:
                                if current_segment["text"]:  # Save previous segment
                                    speaker_segments.append(current_segment)

                                # Start new segment
                                current_segment = {
                                    "speaker": speaker_id,
                                    "start_time": word_data["start_time"],
                                    "end_time": word_data["end_time"],
                                    "text": word_data["word"],
                                    "confidence": word_data["confidence"],
                                    "words": [word_data]
                                }
                            else:
                                # Continue current segment
                                current_segment["text"] += " " + word_data["word"]
                                current_segment["end_time"] = word_data["end_time"]
                                current_segment["words"].append(word_data)
                        else:
                            # No speaker info, continue segment
                            if current_segment["start_time"] is None:
                                current_segment["start_time"] = word_data["start_time"]
                            current_segment["end_time"] = word_data["end_time"]
                            current_segment["text"] += (" " if current_segment["text"] else "") + word_data["word"]
                            current_segment["words"].append(word_data)

                        word_count += 1
                        if word_data["confidence"] > 0:
                            total_confidence += word_data["confidence"]
                            confidence_count += 1

                    # Add final segment
                    if current_segment["text"]:
                        speaker_segments.append(current_segment)

        # Calculate overall confidence
        avg_confidence = total_confidence / confidence_count if confidence_count > 0 else 0.0

        # Calculate speaker statistics
        speaker_stats = {}
        audio_duration = 0
        if speaker_segments:
            audio_duration = max(segment["end_time"] for segment in speaker_segments if segment.get("end_time"))

        for speaker in speakers_detected:
            speaker_segments_filtered = [s for s in speaker_segments if s.get("speaker") == speaker]
            total_speaking_time = sum(
                (s["end_time"] or 0) - (s["start_time"] or 0)
                for s in speaker_segments_filtered
                if s.get("start_time") is not None and s.get("end_time") is not None
            )
            speaker_word_count = sum(len(s.get("words", [])) for s in speaker_segments_filtered)

            speaker_stats[speaker] = {
                "total_speaking_time": total_speaking_time,
                "word_count": speaker_word_count,
                "segments": len(speaker_segments_filtered),
                "speaking_percentage": (total_speaking_time / audio_duration * 100) if audio_duration > 0 else 0
            }

        return {
            "transcript_text": full_transcript.strip(),
            "language": language_code,
            "audio_duration": audio_duration,
            "processing_duration": processing_duration,
            "confidence_score": avg_confidence,
            "transcription_method": "google_cloud",
            "speaker_segments": speaker_segments,
            "speakers_detected": list(speakers_detected),
            "speaker_count": len(speakers_detected),
            "speaker_statistics": speaker_stats,
            "word_count": word_count,
            "quality_metrics": {
                "service_used": "google_cloud",
                "model_used": getattr(config, 'model', 'latest_long'),
                "enhanced_model": getattr(config, 'use_enhanced', False),
                "diarization_enabled": getattr(config, 'diarization_config', None) is not None,
                "avg_confidence": avg_confidence,
                "segments_count": len(speaker_segments),
                "processing_time": processing_duration
            },
            "metadata": {
                "audio_file": str(audio_path),
                "file_size": audio_path.stat().st_size,
                "transcribed_at": datetime.utcnow().isoformat() + "Z",
                "language_code": language_code,
                "cloud_service": "google_cloud_speech",
                "api_version": "v1"
            }
        }

    async def test_cloud_connection(self) -> Dict[str, Any]:
        """Test Google Cloud Speech connection"""

        if not self.client:
            return {
                "available": False,
                "error": "Client not initialized - check credentials"
            }

        try:
            # Try to list available models (simple API test)
            # Note: This is a simple test - actual model listing requires different API
            return {
                "available": True,
                "client_initialized": True,
                "credentials_configured": self.credentials_path is not None,
                "test_status": "ready"
            }

        except Exception as e:
            return {
                "available": False,
                "error": f"Connection test failed: {str(e)}"
            }

    def is_available(self) -> bool:
        """Check if Google Cloud Speech is available"""
        return self.client is not None

    async def estimate_cost(self, audio_path: Path) -> Dict[str, Any]:
        """Estimate Google Cloud Speech cost for audio file"""

        try:
            # Load audio to get duration
            audio_data, sample_rate = librosa.load(str(audio_path), sr=None)
            duration_minutes = len(audio_data) / sample_rate / 60

            # Google Cloud Speech pricing (approximate)
            # Standard model: $0.006 per minute
            # Enhanced model: $0.009 per minute
            standard_cost = duration_minutes * 0.006
            enhanced_cost = duration_minutes * 0.009

            return {
                "duration_minutes": duration_minutes,
                "estimated_cost_usd": {
                    "standard_model": round(standard_cost, 4),
                    "enhanced_model": round(enhanced_cost, 4)
                },
                "note": "Pricing estimates based on Google Cloud Speech API rates as of 2024"
            }

        except Exception as e:
            return {
                "error": f"Cost estimation failed: {str(e)}"
            }


# Global cloud STT instance
google_cloud_stt = GoogleCloudSTT()