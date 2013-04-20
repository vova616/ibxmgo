package ibxmgo

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	UnsupportedSamplingRate = errors.New("Unsupported sampling rate")
)

type IBXM struct {
	module        *Module
	rampBuf       []int32
	channels      []*Channel
	interpolation Interpolation
	sampleRate,
	seqPos, breakSeqPos, row, nextRow, tick,
	speed, tempo, plCount, plChannel int
	globalVol int
	note      *Note
}

func NewIBXM(module *Module, samplingRate int) (*IBXM, error) {
	if samplingRate < 8000 || samplingRate > 128000 {
		return nil, UnsupportedSamplingRate
	}
	this := &IBXM{}
	this.module = module
	this.SetSampleRate(samplingRate)
	this.interpolation = LINEAR
	this.rampBuf = make([]int32, 128)
	this.channels = make([]*Channel, module.numChannels)
	this.globalVol = 0
	this.note = &Note{}
	this.SetSequencePos(0)
	return this, nil
}

/* Set the sampling rate of playback. */
func (this *IBXM) SetSampleRate(rate int) error {
	// Use with Module.c2Rate to adjust the tempo of playback.
	// To play at half speed, multiply both the samplingRate and Module.c2Rate by 2.
	if rate < 8000 || rate > 128000 {
		return UnsupportedSamplingRate
	}
	this.sampleRate = rate
	return nil
}

/* Get the current row position. */
func (this *IBXM) Row() int {
	return this.row
}

/* Get the current pattern position in the sequence. */
func (this *IBXM) SequencePos() int {
	return this.seqPos
}

/* Set the resampling quality to one of
   Channel.NEAREST, Channel.LINEAR, or Channel.SINC. */
func (this *IBXM) SetInterpolation(interpolation Interpolation) {
	this.interpolation = interpolation
}

/* Generate audio.
   The number of samples placed into outputBuf is returned.
   The output buffer length must be at least that returned by getMixBufferLength().
   A "sample" is a pair of 16-bit integer amplitudes, one for each of the stereo channels. */
func (this *IBXM) GetAudio(outputBuf []int32) (samples int, songEnd bool) {

	tickLen := this.calculateTickLen(this.tempo, this.sampleRate)
	// Clear output buffer.
	//
	end := (tickLen + 65) * 4
	for idx := 0; idx < end; idx++ {
		outputBuf[idx] = 0
	}
	// Resample.
	for chanIdx := 0; chanIdx < this.module.numChannels; chanIdx++ {
		chn := this.channels[chanIdx]
		chn.resample(outputBuf, 0, (tickLen+65)*2, this.sampleRate*2, this.interpolation)
		chn.updateSampleIdx(tickLen*2, this.sampleRate*2)
	}
	this.downsample(outputBuf, tickLen+64)
	this.volumeRamp(outputBuf, tickLen)
	songEnd = this.doTick()
	return tickLen, songEnd
}

/* Dump raw audio data */
func (this *IBXM) Dump(w io.Writer) error {
	data := make([]int32, this.MixBufferLength())
	buff := make([]byte, len(data)*2)
	t := this.SequencePos()
	this.SetSequencePos(0)
	defer this.SetSequencePos(t)
	for {
		n, end := this.GetAudio(data)
		n *= 2
		for i := 0; i < n; i++ {
			binary.LittleEndian.PutUint16(buff[i*2:], uint16(data[i]))
		}
		_, e := w.Write(buff[:n*2])
		if e != nil {
			return e
		}
		if end {
			return nil
		}
	}

}

/* Returns the length of the buffer required by getAudio(). */
func (this *IBXM) MixBufferLength() int {
	return (this.calculateTickLen(32, 128000) + 65) * 4
}

func (this *IBXM) calculateTickLen(tempo int, samplingRate int) int {
	return (samplingRate * 5) / (tempo * 2)
}

func (this *IBXM) volumeRamp(mixBuf []int32, tickLen int) {
	rampRate := 256 * 2048 / this.sampleRate
	for idx, a1 := 0, 0; a1 < 256; idx, a1 = idx+2, a1+rampRate {
		a2 := 256 - a1
		mixBuf[idx] = (mixBuf[idx]*int32(a1) + this.rampBuf[idx]*int32(a2)) >> 8
		mixBuf[idx+1] = (mixBuf[idx+1]*int32(a1) + this.rampBuf[idx+1]*int32(a2)) >> 8
	}
	copy(this.rampBuf[:128], mixBuf[tickLen*2:])
}

func (this *IBXM) downsample(buf []int32, count int) {
	// 2:1 downsampling with simple but effective anti-aliasing. Buf must contain count * 2 + 1 stereo samples.
	outLen := count * 2
	for inIdx, outIdx := 0, 0; outIdx < outLen; inIdx, outIdx = inIdx+4, outIdx+2 {
		buf[outIdx] = (buf[inIdx] >> 2) + (buf[inIdx+2] >> 1) + (buf[inIdx+4] >> 2)
		buf[outIdx+1] = (buf[inIdx+1] >> 2) + (buf[inIdx+3] >> 1) + (buf[inIdx+5] >> 2)
	}
}

/* Set the pattern in the sequence to play. The tempo is reset to the default. */
func (this *IBXM) SetSequencePos(pos int) {
	if pos >= this.module.sequenceLength {
		pos = 0
	}
	this.breakSeqPos = pos
	this.nextRow = 0
	this.tick = 1
	this.globalVol = this.module.defaultGVol
	this.speed = 6
	this.tempo = 125
	if this.module.defaultSpeed > 0 {
		this.speed = this.module.defaultSpeed
	}
	if this.module.defaultTempo > 0 {
		this.tempo = this.module.defaultTempo
	}
	this.plCount = -1
	this.plChannel = -1
	for idx := 0; idx < this.module.numChannels; idx++ {
		this.channels[idx] = NewChannel(this.module, idx, &this.globalVol)
	}
	for idx := 0; idx < 128; idx++ {
		this.rampBuf[idx] = 0
	}
	this.doTick()
}

func (this *IBXM) doTick() bool {
	songEnd := false
	this.tick--
	if this.tick <= 0 {
		this.tick = this.speed
		songEnd = this.doRow()
	} else {
		for idx := 0; idx < this.module.numChannels; idx++ {
			this.channels[idx].tick()
		}
	}
	return songEnd
}

/* Returns the song duration in samples at the current sampling rate. */
func (this *IBXM) SongDuration() int {
	duration := 0
	p := this.seqPos
	this.SetSequencePos(0)
	songEnd := false
	for !songEnd {
		duration += this.calculateTickLen(this.tempo, this.sampleRate)
		songEnd = this.doTick()
	}
	this.SetSequencePos(p)
	return duration
}

func (this *IBXM) doRow() bool {
	songEnd := false
	if this.breakSeqPos >= 0 {
		if this.breakSeqPos >= this.module.sequenceLength {
			this.breakSeqPos = 0
			this.nextRow = 0
		}
		for this.module.sequence[this.breakSeqPos] >= this.module.numPatterns {
			this.breakSeqPos++
			if this.breakSeqPos >= this.module.sequenceLength {
				this.breakSeqPos = 0
				this.nextRow = 0
			}
		}
		if this.breakSeqPos <= this.seqPos {
			songEnd = true
		}
		this.seqPos = this.breakSeqPos
		for idx := 0; idx < this.module.numChannels; idx++ {
			this.channels[idx].plRow = 0
		}
		this.breakSeqPos = -1
	}
	pattern := this.module.patterns[this.module.sequence[this.seqPos]]
	this.row = this.nextRow
	if this.row >= pattern.numRows {
		this.row = 0
	}
	this.nextRow = this.row + 1
	if this.nextRow >= pattern.numRows {
		this.breakSeqPos = this.seqPos + 1
		this.nextRow = 0
	}
	noteIdx := this.row * this.module.numChannels
	for chanIdx := 0; chanIdx < this.module.numChannels; chanIdx++ {
		channel := this.channels[chanIdx]
		pattern.getNote(noteIdx+chanIdx, this.note)
		note := this.note
		if note.effect == 0xE {
			note.effect = 0x70 | (note.param >> 4)
			note.param &= 0xF
		}
		if note.effect == 0x93 {
			note.effect = 0xF0 | (note.param >> 4)
			note.param &= 0xF
		}
		if note.effect == 0 && note.param > 0 {
			note.effect = 0x8A
		}

		channel.row(note)
		switch note.effect {
		case 0x81: /* Set Speed. */
			if note.param > 0 {
				this.tick = note.param
				this.speed = note.param
			}
			break
		case 0xB:
			fallthrough
		case 0x82: /* Pattern Jump.*/
			if this.plCount < 0 {
				this.breakSeqPos = note.param
				this.nextRow = 0
			}
			break
		case 0xD:
			fallthrough
		case 0x83: /* Pattern Break.*/
			if this.plCount < 0 {
				this.breakSeqPos = this.seqPos + 1
				this.nextRow = (note.param>>4)*10 + (note.param & 0xF)
			}
			break
		case 0xF: /* Set Speed/Tempo.*/
			if note.param > 0 {
				if note.param < 32 {
					this.tick = note.param
					this.speed = note.param
				} else {
					this.tempo = note.param
				}
			}
			break
		case 0x94: /* Set Tempo.*/
			if note.param > 32 {
				this.tempo = note.param
			}
			break
		case 0x76:
			fallthrough
		case 0xFB: /* Pattern Loop.*/
			if note.param == 0 { /* Set loop marker on this channel. */
				channel.plRow = this.row
			}
			if channel.plRow < this.row { /* Marker valid. Begin looping. */
				if this.plCount < 0 { /* Not already looping, begin. */
					this.plCount = note.param
					this.plChannel = chanIdx
				}
				if this.plChannel == chanIdx { /* Next Loop.*/
					if this.plCount == 0 { /* Loop finished. */
						/* Invalidate current marker. */
						channel.plRow = this.row + 1
					} else { /* Loop and cancel any breaks on this row. */
						this.nextRow = channel.plRow
						this.breakSeqPos = -1
					}
					this.plCount--
				}
			}
			break
		case 0x7E:
			fallthrough
		case 0xFE: /* Pattern Delay.*/
			this.tick = this.speed + this.speed*note.param
			break
		}
	}
	return songEnd
}
