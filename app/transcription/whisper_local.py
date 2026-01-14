"""
WhisperX Local Transcription with Mac M4 Optimization

Implements high-accuracy local transcription using WhisperX with speaker diarization
optimized for Mac M4 architecture with unified memory.
"""
import whisperx
import torch
import torchaudio
import librosa
import numpy as np
from pathlib import Path
from typing import Dict, Any, Optional, List, Tuple
import asyncio
from datetime import datetime
import json
import gc

from app.config import settings


class WhisperXProcessor:
    """WhisperX-based local transcription processor optimized for Mac M4"""

    def __init__(self, model_size: str = "large-v3"):
        self.model_size = model_size
        self.device = self._get_optimal_device()
        self.compute_type = self._get_optimal_compute_type()

        # Model components (loaded lazily)
        self.whisper_model = None
        self.align_model = None
        self.align_metadata = None
        self.diarize_model = None

        # Performance settings for Mac M4
        self.batch_size = 16 if self.device == "mps" else 8
        self.chunk_length = 30  # seconds

    def _get_optimal_device(self) -> str:
        """Determine optimal device for Mac M4"""
        if torch.backends.mps.is_available():
            return "mps"  # Metal Performance Shaders for Mac M4
        elif torch.cuda.is_available():
            return "cuda"
        else:
            return "cpu"

    def _get_optimal_compute_type(self) -> str:
        """Get optimal compute type for device"""
        if self.device == "mps":
            return "float32"  # MPS doesn't support all float16 operations yet
        elif self.device == "cuda":
            return "float16"  # GPU can handle mixed precision
        else:
            return "float32"  # CPU fallback

    async def initialize_models(self, language: str = "en") -> bool:
        """Initialize WhisperX models asynchronously"""
        try:
            print(f"🤖 Initializing WhisperX models on {self.device}...")

            # Load Whisper model
            self.whisper_model = whisperx.load_model(
                self.model_size,
                device=self.device,
                compute_type=self.compute_type,
                language=language
            )
            print(f"✅ Whisper {self.model_size} model loaded")

            # Load alignment model for word-level timestamps
            self.align_model, self.align_metadata = whisperx.load_align_model(
                language_code=language,
                device=self.device
            )
            print(f"✅ Alignment model loaded for {language}")

            # Load diarization model for speaker identification
            self.diarize_model = whisperx.DiarizationPipeline(
                use_auth_token=None,  # Will use default model
                device=self.device
            )
            print(f"✅ Diarization model loaded")

            return True

        except Exception as e:
            print(f"❌ Error initializing models: {str(e)}")
            return False

    async def transcribe_audio(
        self,
        audio_path: Path,
        language: str = "en",
        min_speakers: Optional[int] = None,
        max_speakers: Optional[int] = None
    ) -> Dict[str, Any]:
        """Transcribe audio file with speaker diarization"""

        if not audio_path.exists():
            raise FileNotFoundError(f"Audio file not found: {audio_path}")

        try:
            # Ensure models are loaded
            if not self.whisper_model:
                await self.initialize_models(language)

            start_time = datetime.utcnow()
            print(f"🎤 Starting transcription of {audio_path.name}...")

            # Load and preprocess audio
            audio_data = await self._load_and_preprocess_audio(audio_path)
            print(f"📁 Audio loaded: {len(audio_data)/16000:.1f}s duration")

            # Step 1: Transcribe with Whisper
            print("🔤 Running Whisper transcription...")
            transcribe_result = self.whisper_model.transcribe(
                audio_data,
                batch_size=self.batch_size,
                language=language
            )

            # Step 2: Align whisper output for word-level timestamps
            print("🎯 Aligning word-level timestamps...")
            align_result = whisperx.align(
                transcribe_result["segments"],
                self.align_model,
                self.align_metadata,
                audio_data,
                self.device,
                return_char_alignments=False
            )

            # Step 3: Assign speaker labels
            print("👥 Running speaker diarization...")
            diarize_segments = self.diarize_model(
                audio_path,
                min_speakers=min_speakers,
                max_speakers=max_speakers
            )

            # Assign speakers to segments
            final_result = whisperx.assign_word_speakers(
                diarize_segments,
                align_result
            )

            # Calculate processing metrics
            end_time = datetime.utcnow()
            processing_duration = (end_time - start_time).total_seconds()
            audio_duration = len(audio_data) / 16000
            speed_factor = audio_duration / processing_duration

            print(f"✅ Transcription complete in {processing_duration:.1f}s ({speed_factor:.1f}x realtime)")

            # Build comprehensive result
            result = await self._build_transcription_result(
                final_result,
                audio_path,
                audio_duration,
                processing_duration,
                language
            )

            # Cleanup memory
            if self.device == "mps":
                torch.mps.empty_cache()
            elif self.device == "cuda":
                torch.cuda.empty_cache()
            gc.collect()

            return result

        except Exception as e:
            print(f"❌ Transcription failed: {str(e)}")
            raise RuntimeError(f"Transcription failed: {str(e)}")

    async def _load_and_preprocess_audio(self, audio_path: Path) -> np.ndarray:
        """Load and preprocess audio for optimal transcription"""

        try:
            # Load audio file
            audio_data = whisperx.load_audio(str(audio_path))

            # Ensure sample rate is 16kHz (Whisper requirement)
            if len(audio_data.shape) > 1:
                # Convert stereo to mono
                audio_data = librosa.to_mono(audio_data)

            # Normalize audio
            audio_data = librosa.util.normalize(audio_data)

            return audio_data

        except Exception as e:
            raise ValueError(f"Error loading audio file {audio_path}: {str(e)}")

    async def _build_transcription_result(
        self,
        whisperx_result: Dict[str, Any],
        audio_path: Path,
        audio_duration: float,
        processing_duration: float,
        language: str
    ) -> Dict[str, Any]:
        """Build comprehensive transcription result"""

        segments = whisperx_result.get("segments", [])

        # Extract speakers and calculate stats
        speakers = set()
        word_count = 0
        total_confidence = 0
        confidence_count = 0

        for segment in segments:
            if segment.get("speaker"):
                speakers.add(segment["speaker"])

            # Count words and aggregate confidence
            words = segment.get("words", [])
            word_count += len(words)

            for word in words:
                if "score" in word:
                    total_confidence += word["score"]
                    confidence_count += 1

        # Calculate average confidence
        avg_confidence = total_confidence / confidence_count if confidence_count > 0 else 0.0

        # Build full transcript text
        full_transcript = " ".join([segment.get("text", "").strip() for segment in segments])

        # Extract speaker segments for easier processing
        speaker_segments = []
        for segment in segments:
            speaker_segments.append({
                "speaker": segment.get("speaker", "Unknown"),
                "start_time": segment.get("start", 0),
                "end_time": segment.get("end", 0),
                "text": segment.get("text", "").strip(),
                "confidence": segment.get("avg_logprob", 0) if "avg_logprob" in segment else None,
                "words": segment.get("words", [])
            })

        # Calculate speaker statistics
        speaker_stats = {}
        for speaker in speakers:
            speaker_segments_filtered = [s for s in speaker_segments if s["speaker"] == speaker]
            total_speaking_time = sum(
                s["end_time"] - s["start_time"] for s in speaker_segments_filtered
            )
            speaker_word_count = sum(len(s["words"]) for s in speaker_segments_filtered)

            speaker_stats[speaker] = {
                "total_speaking_time": total_speaking_time,
                "word_count": speaker_word_count,
                "segments": len(speaker_segments_filtered),
                "speaking_percentage": (total_speaking_time / audio_duration) * 100
            }

        return {
            "transcript_text": full_transcript,
            "language": language,
            "audio_duration": audio_duration,
            "processing_duration": processing_duration,
            "speed_factor": audio_duration / processing_duration,
            "confidence_score": avg_confidence,
            "transcription_method": f"whisperx_{self.model_size}_local",
            "speaker_segments": speaker_segments,
            "speakers_detected": list(speakers),
            "speaker_count": len(speakers),
            "speaker_statistics": speaker_stats,
            "word_count": word_count,
            "quality_metrics": {
                "device_used": self.device,
                "model_size": self.model_size,
                "batch_size": self.batch_size,
                "avg_confidence": avg_confidence,
                "segments_count": len(segments),
                "processing_speed": f"{audio_duration / processing_duration:.1f}x"
            },
            "metadata": {
                "audio_file": str(audio_path),
                "file_size": audio_path.stat().st_size,
                "transcribed_at": datetime.utcnow().isoformat() + "Z",
                "whisperx_version": whisperx.__version__ if hasattr(whisperx, "__version__") else "unknown",
                "torch_version": torch.__version__
            }
        }

    async def get_model_info(self) -> Dict[str, Any]:
        """Get information about loaded models"""
        return {
            "whisper_model_size": self.model_size,
            "device": self.device,
            "compute_type": self.compute_type,
            "batch_size": self.batch_size,
            "models_loaded": {
                "whisper": self.whisper_model is not None,
                "alignment": self.align_model is not None,
                "diarization": self.diarize_model is not None
            },
            "memory_usage": self._get_memory_usage()
        }

    def _get_memory_usage(self) -> Dict[str, float]:
        """Get current memory usage"""
        try:
            if self.device == "mps":
                # MPS memory tracking
                return {
                    "allocated_mb": torch.mps.current_allocated_memory() / 1024 / 1024,
                    "reserved_mb": torch.mps.driver_allocated_memory() / 1024 / 1024
                }
            elif self.device == "cuda":
                return {
                    "allocated_mb": torch.cuda.memory_allocated() / 1024 / 1024,
                    "reserved_mb": torch.cuda.memory_reserved() / 1024 / 1024
                }
            else:
                return {"allocated_mb": 0, "reserved_mb": 0}
        except:
            return {"allocated_mb": 0, "reserved_mb": 0}

    async def cleanup_models(self):
        """Clean up models to free memory"""
        self.whisper_model = None
        self.align_model = None
        self.align_metadata = None
        self.diarize_model = None

        # Force garbage collection
        if self.device == "mps":
            torch.mps.empty_cache()
        elif self.device == "cuda":
            torch.cuda.empty_cache()
        gc.collect()

    async def test_transcription(self, test_audio_path: Optional[Path] = None) -> Dict[str, Any]:
        """Test transcription functionality"""

        if test_audio_path and test_audio_path.exists():
            try:
                result = await self.transcribe_audio(test_audio_path)
                return {
                    "test_status": "success",
                    "test_file": str(test_audio_path),
                    "transcript_length": len(result["transcript_text"]),
                    "speakers_detected": result["speaker_count"],
                    "processing_speed": result["speed_factor"],
                    "confidence": result["confidence_score"]
                }
            except Exception as e:
                return {
                    "test_status": "failed",
                    "error": str(e)
                }
        else:
            # Test model loading only
            try:
                await self.initialize_models()
                model_info = await self.get_model_info()
                return {
                    "test_status": "models_loaded",
                    "model_info": model_info
                }
            except Exception as e:
                return {
                    "test_status": "failed",
                    "error": str(e)
                }


# Global processor instance
whisper_processor = WhisperXProcessor(model_size=settings.WHISPER_MODEL_SIZE)