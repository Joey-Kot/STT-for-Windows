//! A fresh detector per input; streaming 16 kHz mono signed PCM, 256 samples/frame.
use crate::{audio_intervals::AudioInterval, converter::ConvertError};

const FRAME: usize = 256;
const START_FRAMES: u64 = 2;
const MIN_VOICE_FRAMES: u64 = 4;
const END_SILENCE_FRAMES: u64 = 10;

pub struct Vad {
    detector: earshot::Detector,
    pending: [i16; FRAME],
    used: usize,
    position: u64,
    candidate: Option<u64>,
    voiced: u64,
    consecutive: u64,
    active: bool,
    last_voice_end: u64,
    intervals: Vec<AudioInterval>,
}
impl Default for Vad {
    fn default() -> Self {
        Self {
            detector: earshot::Detector::default(),
            pending: [0; FRAME],
            used: 0,
            position: 0,
            candidate: None,
            voiced: 0,
            consecutive: 0,
            active: false,
            last_voice_end: 0,
            intervals: Vec::new(),
        }
    }
}
impl Vad {
    pub fn push(&mut self, mut samples: &[i16]) -> Result<(), ConvertError> {
        while !samples.is_empty() {
            let n = (FRAME - self.used).min(samples.len());
            self.pending[self.used..self.used + n].copy_from_slice(&samples[..n]);
            self.used += n;
            samples = &samples[n..];
            if self.used == FRAME {
                let voice = self.detector.predict_i16(&self.pending) >= 0.5;
                self.accept(voice, FRAME as u64)?;
                self.used = 0;
            }
        }
        Ok(())
    }
    fn accept(&mut self, voice: bool, samples: u64) -> Result<(), ConvertError> {
        let start = self.position;
        self.position = self
            .position
            .checked_add(samples)
            .ok_or_else(|| ConvertError::Failed {
                message: "VAD position overflow".into(),
            })?;
        if voice {
            self.candidate.get_or_insert(start);
            self.voiced += 1;
            self.consecutive += 1;
            self.last_voice_end = self.position;
            self.active |= self.consecutive >= START_FRAMES;
        } else {
            self.consecutive = 0;
            if !self.active {
                self.candidate = None;
                self.voiced = 0;
            } else if self.position - self.last_voice_end >= END_SILENCE_FRAMES * FRAME as u64 {
                self.close()?;
            }
        }
        Ok(())
    }
    fn close(&mut self) -> Result<(), ConvertError> {
        if self.active && self.voiced >= MIN_VOICE_FRAMES {
            self.intervals
                .try_reserve(1)
                .map_err(|_| ConvertError::Failed {
                    message: "VAD interval allocation failed".into(),
                })?;
            self.intervals.push(AudioInterval {
                start_frame: self.candidate.unwrap(),
                end_frame: self.last_voice_end,
            });
        }
        self.candidate = None;
        self.voiced = 0;
        self.consecutive = 0;
        self.active = false;
        Ok(())
    }
    pub fn finish(mut self) -> Result<Vec<AudioInterval>, ConvertError> {
        if self.used > 0 {
            self.pending[self.used..].fill(0);
            let voice = self.detector.predict_i16(&self.pending) >= 0.5;
            self.accept(voice, self.used as u64)?;
        }
        self.close()?;
        Ok(self.intervals)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn silence_and_partial_tail() {
        let mut vad = Vad::default();
        for _ in 0..100 {
            vad.push(&[0; 317]).unwrap();
        }
        assert!(vad.finish().unwrap().is_empty());
    }
    #[test]
    fn hysteresis_flush_and_short_bursts() {
        let mut vad = Vad::default();
        vad.accept(true, 256).unwrap();
        vad.accept(false, 256).unwrap();
        for _ in 0..4 {
            vad.accept(true, 256).unwrap();
        }
        for _ in 0..5 {
            vad.accept(false, 256).unwrap();
        }
        vad.accept(true, 17).unwrap();
        assert_eq!(
            vad.finish().unwrap(),
            vec![AudioInterval {
                start_frame: 512,
                end_frame: 2833
            }]
        );
        let mut vad = Vad::default();
        vad.accept(true, 17).unwrap();
        assert!(vad.finish().unwrap().is_empty());
    }
}
