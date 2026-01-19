"""
Audio Quality Assessment for Transcription Decision

Analyzes audio quality to determine whether to use local WhisperX or
escalate to cloud STT services for better accuracy.
"""
import librosa
import numpy as np
from pathlib import Path
from typing import Dict, Any, Tuple, Optional
import scipy.signal
from scipy.stats import entropy
import math

from app.config import settings


class AudioQualityAnalyzer:
    """Analyzes audio quality for transcription routing decisions"""

    def __init__(self):
        # Quality thresholds from research.md
        self.snr_threshold = 15.0  # dB - below this, escalate to cloud
        self.confidence_threshold = settings.STT_CONFIDENCE_THRESHOLD  # 0.8
        self.spectral_rolloff_threshold = 0.85
        self.zero_crossing_rate_threshold = 0.1

    async def analyze_audio_quality(self, audio_path: Path) -> Dict[str, Any]:
        """Comprehensive audio quality analysis"""

        if not audio_path.exists():
            raise FileNotFoundError(f"Audio file not found: {audio_path}")

        try:
            # Load audio
            audio_data, sample_rate = librosa.load(str(audio_path), sr=None)

            # Run quality assessments
            quality_metrics = {}

            # 1. Signal-to-Noise Ratio
            quality_metrics.update(await self._calculate_snr(audio_data, sample_rate))

            # 2. Spectral Analysis
            quality_metrics.update(await self._analyze_spectral_features(audio_data, sample_rate))

            # 3. Dynamic Range Analysis
            quality_metrics.update(await self._analyze_dynamic_range(audio_data))

            # 4. Voice Activity Detection
            quality_metrics.update(await self._detect_voice_activity(audio_data, sample_rate))

            # 5. Audio Characteristics
            quality_metrics.update(await self._analyze_audio_characteristics(audio_data, sample_rate))

            # 6. Overall Quality Score
            overall_score = await self._calculate_overall_quality_score(quality_metrics)

            # 7. Routing Decision
            routing_decision = await self._make_routing_decision(quality_metrics, overall_score)

            return {
                "audio_file": str(audio_path),
                "file_size": audio_path.stat().st_size,
                "duration_seconds": len(audio_data) / sample_rate,
                "sample_rate": sample_rate,
                "channels": 1 if len(audio_data.shape) == 1 else audio_data.shape[0],
                "quality_metrics": quality_metrics,
                "overall_quality_score": overall_score,
                "routing_decision": routing_decision,
                "analysis_timestamp": None  # Will be set by caller
            }

        except Exception as e:
            return {
                "error": f"Quality analysis failed: {str(e)}",
                "routing_decision": {
                    "use_local": False,
                    "reason": "analysis_failed",
                    "recommended_service": "google_cloud"
                }
            }

    async def _calculate_snr(self, audio_data: np.ndarray, sample_rate: int) -> Dict[str, float]:
        """Calculate Signal-to-Noise Ratio"""

        # Use spectral subtraction method for SNR estimation
        # Split audio into segments
        segment_length = int(sample_rate * 0.5)  # 0.5 second segments
        segments = []

        for i in range(0, len(audio_data) - segment_length, segment_length):
            segment = audio_data[i:i + segment_length]
            segments.append(segment)

        if not segments:
            return {"snr_db": 0.0, "snr_category": "poor"}

        # Calculate power for each segment
        segment_powers = [np.mean(segment**2) for segment in segments]

        # Estimate noise floor (bottom 25% of segments)
        sorted_powers = sorted(segment_powers)
        noise_power = np.mean(sorted_powers[:len(sorted_powers)//4])

        # Estimate signal power (top 25% of segments)
        signal_power = np.mean(sorted_powers[3*len(sorted_powers)//4:])

        # Calculate SNR
        if noise_power > 0:
            snr_db = 10 * math.log10(signal_power / noise_power)
        else:
            snr_db = 60.0  # Very clean signal

        # Categorize SNR
        if snr_db >= 20:
            snr_category = "excellent"
        elif snr_db >= 15:
            snr_category = "good"
        elif snr_db >= 10:
            snr_category = "fair"
        else:
            snr_category = "poor"

        return {
            "snr_db": snr_db,
            "snr_category": snr_category,
            "noise_power": float(noise_power),
            "signal_power": float(signal_power)
        }

    async def _analyze_spectral_features(self, audio_data: np.ndarray, sample_rate: int) -> Dict[str, float]:
        """Analyze spectral characteristics"""

        # Compute spectral features
        spectral_centroids = librosa.feature.spectral_centroid(y=audio_data, sr=sample_rate)[0]
        spectral_rolloff = librosa.feature.spectral_rolloff(y=audio_data, sr=sample_rate, roll_percent=0.85)[0]
        spectral_bandwidth = librosa.feature.spectral_bandwidth(y=audio_data, sr=sample_rate)[0]
        zero_crossing_rate = librosa.feature.zero_crossing_rate(audio_data)[0]

        # Calculate statistics
        return {
            "spectral_centroid_mean": float(np.mean(spectral_centroids)),
            "spectral_centroid_std": float(np.std(spectral_centroids)),
            "spectral_rolloff_mean": float(np.mean(spectral_rolloff)),
            "spectral_rolloff_std": float(np.std(spectral_rolloff)),
            "spectral_bandwidth_mean": float(np.mean(spectral_bandwidth)),
            "zero_crossing_rate_mean": float(np.mean(zero_crossing_rate)),
            "zero_crossing_rate_std": float(np.std(zero_crossing_rate))
        }

    async def _analyze_dynamic_range(self, audio_data: np.ndarray) -> Dict[str, float]:
        """Analyze dynamic range and amplitude characteristics"""

        # Calculate RMS energy
        rms = librosa.feature.rms(y=audio_data)[0]

        # Peak and RMS ratios
        peak_amplitude = np.max(np.abs(audio_data))
        rms_amplitude = np.sqrt(np.mean(audio_data**2))

        # Dynamic range metrics
        dynamic_range = 20 * math.log10(peak_amplitude / (rms_amplitude + 1e-8))

        # Clipping detection
        clipping_ratio = np.sum(np.abs(audio_data) > 0.95) / len(audio_data)

        return {
            "peak_amplitude": float(peak_amplitude),
            "rms_amplitude": float(rms_amplitude),
            "dynamic_range_db": float(dynamic_range),
            "rms_mean": float(np.mean(rms)),
            "rms_std": float(np.std(rms)),
            "clipping_ratio": float(clipping_ratio)
        }

    async def _detect_voice_activity(self, audio_data: np.ndarray, sample_rate: int) -> Dict[str, Any]:
        """Detect voice activity and speech characteristics"""

        # Frame-based voice activity detection
        frame_length = int(sample_rate * 0.025)  # 25ms frames
        hop_length = int(sample_rate * 0.010)   # 10ms hop

        # Calculate short-time energy
        frames = librosa.util.frame(audio_data, frame_length=frame_length,
                                  hop_length=hop_length, axis=0)
        energy = np.array([np.sum(frame**2) for frame in frames])

        # Voice activity detection using energy threshold
        energy_threshold = np.percentile(energy, 50)  # Median energy as threshold
        voice_frames = energy > energy_threshold

        voice_activity_ratio = np.sum(voice_frames) / len(voice_frames)

        # Estimate number of speech segments
        voice_transitions = np.diff(voice_frames.astype(int))
        speech_segments = np.sum(voice_transitions == 1)  # Start of speech segments

        return {
            "voice_activity_ratio": float(voice_activity_ratio),
            "speech_segments_estimated": int(speech_segments),
            "energy_variance": float(np.var(energy)),
            "energy_mean": float(np.mean(energy))
        }

    async def _analyze_audio_characteristics(self, audio_data: np.ndarray, sample_rate: int) -> Dict[str, Any]:
        """Analyze general audio characteristics"""

        # Frequency analysis
        fft = np.fft.fft(audio_data)
        freqs = np.fft.fftfreq(len(fft), 1/sample_rate)
        magnitude = np.abs(fft)

        # Find dominant frequency
        dominant_freq_idx = np.argmax(magnitude[:len(magnitude)//2])
        dominant_frequency = freqs[dominant_freq_idx]

        # Spectral entropy (measure of spectral complexity)
        # Normalize magnitude spectrum
        magnitude_normalized = magnitude / np.sum(magnitude)
        spectral_entropy = entropy(magnitude_normalized + 1e-12)

        # Harmonic analysis
        harmonic_ratio = await self._calculate_harmonic_ratio(audio_data, sample_rate)

        return {
            "dominant_frequency": float(abs(dominant_frequency)),
            "spectral_entropy": float(spectral_entropy),
            "harmonic_ratio": harmonic_ratio,
            "audio_complexity": "high" if spectral_entropy > 10 else "low"
        }

    async def _calculate_harmonic_ratio(self, audio_data: np.ndarray, sample_rate: int) -> float:
        """Calculate harmonic-to-noise ratio"""

        try:
            # Use harmonic-percussive separation
            harmonic, percussive = librosa.effects.hpss(audio_data)

            # Calculate power ratio
            harmonic_power = np.mean(harmonic**2)
            percussive_power = np.mean(percussive**2)

            if percussive_power > 0:
                ratio = harmonic_power / percussive_power
                return float(10 * math.log10(ratio + 1e-8))
            else:
                return 20.0  # Very harmonic

        except Exception:
            return 0.0  # Fallback

    async def _calculate_overall_quality_score(self, metrics: Dict[str, Any]) -> float:
        """Calculate overall quality score (0.0 to 1.0)"""

        score = 0.0
        weight_sum = 0.0

        # SNR contribution (30% weight)
        snr_db = metrics.get("snr_db", 0)
        snr_score = min(1.0, max(0.0, (snr_db - 5) / 20))  # Normalize 5-25 dB to 0-1
        score += snr_score * 0.3
        weight_sum += 0.3

        # Voice activity contribution (20% weight)
        voice_ratio = metrics.get("voice_activity_ratio", 0)
        voice_score = min(1.0, voice_ratio * 2)  # Good if >50% voice activity
        score += voice_score * 0.2
        weight_sum += 0.2

        # Dynamic range contribution (20% weight)
        dynamic_range = metrics.get("dynamic_range_db", 0)
        dynamic_score = min(1.0, max(0.0, (dynamic_range - 10) / 30))  # 10-40 dB range
        score += dynamic_score * 0.2
        weight_sum += 0.2

        # Clipping penalty (15% weight)
        clipping_ratio = metrics.get("clipping_ratio", 0)
        clipping_score = max(0.0, 1.0 - clipping_ratio * 10)  # Penalize clipping heavily
        score += clipping_score * 0.15
        weight_sum += 0.15

        # Spectral quality (15% weight)
        spectral_entropy = metrics.get("spectral_entropy", 15)
        spectral_score = min(1.0, max(0.0, 1.0 - (spectral_entropy - 8) / 10))
        score += spectral_score * 0.15
        weight_sum += 0.15

        return score / weight_sum if weight_sum > 0 else 0.0

    async def _make_routing_decision(
        self,
        metrics: Dict[str, Any],
        overall_score: float
    ) -> Dict[str, Any]:
        """Make routing decision based on quality analysis"""

        # Primary decision factors
        snr_db = metrics.get("snr_db", 0)
        voice_activity = metrics.get("voice_activity_ratio", 0)
        clipping_ratio = metrics.get("clipping_ratio", 0)

        # Decision logic
        use_local = True
        reason = "good_quality"
        confidence = overall_score

        # Check for disqualifying factors
        if snr_db < self.snr_threshold:
            use_local = False
            reason = "low_snr"
        elif voice_activity < 0.3:
            use_local = False
            reason = "low_voice_activity"
        elif clipping_ratio > 0.1:
            use_local = False
            reason = "excessive_clipping"
        elif overall_score < 0.4:
            use_local = False
            reason = "poor_overall_quality"

        # Determine recommended service
        if use_local:
            recommended_service = "whisperx_local"
        else:
            # Choose cloud service based on specific issues
            if snr_db < 10 or overall_score < 0.3:
                recommended_service = "google_cloud"  # Better noise handling
            else:
                recommended_service = "google_cloud"  # Default cloud fallback

        return {
            "use_local": use_local,
            "reason": reason,
            "confidence": confidence,
            "recommended_service": recommended_service,
            "quality_summary": {
                "snr_acceptable": snr_db >= self.snr_threshold,
                "voice_activity_sufficient": voice_activity >= 0.3,
                "clipping_minimal": clipping_ratio <= 0.1,
                "overall_score": overall_score
            }
        }

    async def quick_quality_check(self, audio_path: Path) -> Dict[str, Any]:
        """Quick quality check for fast routing decisions"""

        try:
            # Load only first 30 seconds for quick analysis
            audio_data, sample_rate = librosa.load(str(audio_path), duration=30.0, sr=None)

            # Quick SNR estimation
            segment_length = min(len(audio_data), sample_rate)  # 1 second max
            audio_segment = audio_data[:segment_length]

            # Simple energy-based SNR
            energy = np.mean(audio_segment**2)
            noise_floor = np.percentile(audio_segment**2, 10)  # Bottom 10% as noise

            if noise_floor > 0:
                quick_snr = 10 * math.log10(energy / noise_floor)
            else:
                quick_snr = 30.0

            # Quick voice activity
            voice_frames = np.abs(audio_segment) > np.percentile(np.abs(audio_segment), 70)
            voice_ratio = np.sum(voice_frames) / len(voice_frames)

            # Quick decision
            use_local = quick_snr >= self.snr_threshold and voice_ratio >= 0.3

            return {
                "quick_snr_db": quick_snr,
                "quick_voice_ratio": voice_ratio,
                "use_local": use_local,
                "recommended_service": "whisperx_local" if use_local else "google_cloud",
                "analysis_type": "quick"
            }

        except Exception as e:
            return {
                "error": f"Quick analysis failed: {str(e)}",
                "use_local": False,
                "recommended_service": "google_cloud",
                "analysis_type": "quick"
            }


# Global analyzer instance
audio_quality_analyzer = AudioQualityAnalyzer()