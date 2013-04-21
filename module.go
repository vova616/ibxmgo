package ibxmgo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	//	"fmt"
	"io"
	"io/ioutil"
)

type C2Rate int

const (
	NTSC = C2Rate(8363)
	PAL  = C2Rate(8287)

	FP_SHIFT = 15
	FP_ONE   = 1 << FP_SHIFT
	FP_MASK  = FP_ONE - 1

	/* Constants for the 16-tap fixed-point sinc interpolator. */
	LOG2_FILTER_TAPS    = 4
	FILTER_TAPS         = 1 << LOG2_FILTER_TAPS
	DELAY               = FILTER_TAPS / 2
	LOG2_TABLE_ACCURACY = 4
	TABLE_ACCURACY      = 1 << LOG2_TABLE_ACCURACY
	TABLE_INTERP_SHIFT  = FP_SHIFT - LOG2_TABLE_ACCURACY
	TABLE_INTERP_ONE    = 1 << TABLE_INTERP_SHIFT
	TABLE_INTERP_MASK   = TABLE_INTERP_ONE - 1
)

var (
	xmHeader          = []byte("Extended Module: ")
	formats           []format
	deltaEnvHeader    = []byte("DigiBooster Pro")
	UnsupportedFormat = errors.New("Unsupported format")

	keyToPeriod = []int{
		29020, 27392, 25855, 24403, 23034, 21741, 20521,
		19369, 18282, 17256, 16287, 15373, 14510, 13696,
	}
)

type format struct {
	name   string
	check  func(r *bufio.Reader) bool
	decode func(r *bufio.Reader) (*Module, error)
}

type Instrument struct {
	name string

	vibratoType, vibratoSweep, vibratoDepth, vibratoRate int
	volumeFadeOut                                        int

	numSamples      int
	samples         []*Sample
	keyToSample     [97]int
	volumeEnvelope  *Envelope
	panningEnvelope *Envelope
}

func DefaultInstrument() *Instrument {
	return &Instrument{
		numSamples:      1,
		volumeEnvelope:  DefaultEnvelope(),
		panningEnvelope: DefaultEnvelope(),
		samples:         []*Sample{&Sample{}},
	}
}

type Module struct {
	songName                                      string
	numChannels, numInstruments                   int
	numPatterns, sequenceLength, restartPos       int
	defaultGVol, defaultSpeed, defaultTempo, gain int
	c2Rate                                        C2Rate
	linearPeriods, fastVolSlides                  bool
	defaultPanning                                []int
	sequence                                      []int
	patterns                                      []*Pattern
	instruments                                   []*Instrument
}

func NewModule() *Module {
	return &Module{
		songName:       "Blank",
		numChannels:    4,
		numInstruments: 1,
		numPatterns:    1,
		sequenceLength: 1,
		defaultGVol:    64,
		defaultSpeed:   6,
		defaultTempo:   125,
		c2Rate:         PAL,
		gain:           64,
		instruments:    []*Instrument{DefaultInstrument(), DefaultInstrument()},
		patterns:       []*Pattern{NewPattern(4, 64)},
		sequence:       []int{0},
		defaultPanning: []int{51, 204, 204, 51},
	}
}

func RegisterFormat(name string, check func(r *bufio.Reader) bool, decode func(r *bufio.Reader) (*Module, error)) {
	formats = append(formats, format{name, check, decode})
}

func init() {
	RegisterFormat("xm", IsXM, DecodeXM)
	RegisterFormat("mod", IsMOD, DecodeMOD)
	RegisterFormat("s3m", IsS3M, DecodeS3M)
}

func Decode(r io.Reader) (*Module, error) {
	reader := bufio.NewReader(r)
	for _, f := range formats {
		if f.check(reader) {
			return f.decode(reader)
		}
	}
	return nil, UnsupportedFormat
}

func IsXM(reader *bufio.Reader) bool {
	header, e := reader.Peek(17)
	if e != nil {
		return false
	}
	return bytes.Equal(header[0:17], xmHeader)
}

func IsMOD(reader *bufio.Reader) bool {
	header, e := reader.Peek(1084)
	if e != nil {
		return false
	}
	modFormat := binary.BigEndian.Uint16(header[1082:])
	return modFormat == 0x4b2e || modFormat == 0x4b21 || modFormat == 0x5434 ||
		modFormat == 0x484e || modFormat == 0x4348
}

func IsS3M(reader *bufio.Reader) bool {
	header, e := reader.Peek(48)
	if e != nil {
		return false
	}

	return binary.LittleEndian.Uint32(header[44:]) == 0x4d524353
}

func DecodeS3M(reader *bufio.Reader) (*Module, error) {
	buff, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	m := NewModule()

	m.songName = string(buff[0:28])
	m.sequenceLength = int(binary.LittleEndian.Uint16(buff[32:]))
	m.numInstruments = int(binary.LittleEndian.Uint16(buff[34:]))
	m.numPatterns = int(binary.LittleEndian.Uint16(buff[36:]))
	flags := binary.LittleEndian.Uint16(buff[38:])
	version := binary.LittleEndian.Uint16(buff[40:])
	m.fastVolSlides = ((flags & 0x40) == 0x40) || version == 0x1300
	signedSamples := binary.LittleEndian.Uint16(buff[42:]) == 1
	if binary.LittleEndian.Uint32(buff[44:]) != 0x4d524353 {
		errors.New("Not an S3M file!")
	}
	m.defaultGVol = int(buff[48])
	m.defaultSpeed = int(buff[49])
	m.defaultTempo = int(buff[50])
	m.c2Rate = NTSC
	m.gain = int(buff[51] & 0x7F)
	stereoMode := (buff[51] & 0x80) == 0x80
	defaultPan := (buff[53] & 0xFF) == 0xFC
	channelMap := make([]int, 32)
	for chanIdx := 0; chanIdx < 32; chanIdx++ {
		channelMap[chanIdx] = -1
		if buff[64+chanIdx] < 16 {
			channelMap[chanIdx] = m.numChannels
			m.numChannels++
		}
	}
	m.sequence = make([]int, m.sequenceLength)
	for seqIdx := 0; seqIdx < m.sequenceLength; seqIdx++ {
		m.sequence[seqIdx] = int(buff[96+seqIdx])
	}
	moduleDataIdx := 96 + m.sequenceLength
	m.instruments = make([]*Instrument, m.numInstruments+1)
	m.instruments[0] = DefaultInstrument()
	for instIdx := 1; instIdx <= m.numInstruments; instIdx++ {
		instrument := DefaultInstrument()
		m.instruments[instIdx] = instrument
		sample := instrument.samples[0]
		instOffset := int(binary.LittleEndian.Uint16(buff[moduleDataIdx:])) << 4
		moduleDataIdx += 2
		instrument.name = string(buff[instOffset+48 : instOffset+48+28])
		if buff[instOffset] != 1 {
			continue
		}
		if binary.LittleEndian.Uint16(buff[instOffset+76:]) != 0x4353 {
			continue
		}
		sampleOffset := int(buff[instOffset+13]) << 20
		sampleOffset += int(binary.LittleEndian.Uint16(buff[instOffset+14:])) << 4
		sampleLength := int(binary.LittleEndian.Uint32(buff[instOffset+16:]))
		loopStart := int(binary.LittleEndian.Uint32(buff[instOffset+20:]))
		loopLength := int(binary.LittleEndian.Uint32(buff[instOffset+24:])) - loopStart
		sample.volume = int(buff[instOffset+28])
		sample.panning = -1
		packed := buff[instOffset+30] != 0
		loopOn := (buff[instOffset+31] & 0x1) == 0x1
		if loopStart+loopLength > sampleLength {
			loopLength = sampleLength - loopStart
		}
		if loopLength < 1 || !loopOn {
			loopStart = sampleLength
			loopLength = 0
		}
		stereo := (buff[instOffset+31] & 0x2) == 0x2
		_ = stereo
		sixteenBit := (buff[instOffset+31] & 0x4) == 0x4
		if packed {
			errors.New("Packed samples not supported!")
		}
		sample.c2Rate = C2Rate(binary.LittleEndian.Uint32(buff[instOffset+32:]))
		sampleData := make([]int16, loopStart+loopLength)
		if sixteenBit {
			if signedSamples {
				for idx, end := 0, len(sampleData); idx < end; idx++ {
					sampleData[idx] = (int16)(int16(buff[sampleOffset]) | (int16(buff[sampleOffset+1]) << 8))
					sampleOffset += 2
				}
			} else {
				for idx, end := 0, len(sampleData); idx < end; idx++ {
					sam := int(buff[sampleOffset]) | (int(buff[sampleOffset+1]) << 8)
					sampleData[idx] = (int16)(sam - 32768)
					sampleOffset += 2
				}
			}
		} else {
			if signedSamples {
				for idx, end := 0, len(sampleData); idx < end; idx++ {
					sampleData[idx] = (int16)(buff[sampleOffset]) << 8
					sampleOffset++
				}
			} else {
				for idx, end := 0, len(sampleData); idx < end; idx++ {
					sampleData[idx] = (int16)((int(buff[sampleOffset]) - 128) << 8)
					sampleOffset++
				}
			}
		}
		sample.setSampleData(sampleData, loopStart, loopLength, false)
	}
	m.patterns = make([]*Pattern, m.numPatterns)
	for patIdx := 0; patIdx < m.numPatterns; patIdx++ {
		pattern := NewPattern(m.numChannels, 64)
		m.patterns[patIdx] = pattern
		inOffset := (binary.LittleEndian.Uint16(buff[moduleDataIdx:]) << 4) + 2
		rowIdx := 0
		for rowIdx < 64 {
			token := buff[inOffset]
			inOffset++
			if token == 0 {
				rowIdx++
				continue
			}
			noteKey := 0
			noteIns := 0
			if (token & 0x20) == 0x20 { /* Key + Instrument.*/
				noteKey = int(buff[inOffset])
				inOffset++
				noteIns = int(buff[inOffset])
				inOffset++
				if noteKey < 0xFE {
					noteKey = (noteKey>>4)*12 + (noteKey & 0xF) + 1
				}
				if noteKey == 0xFF {
					noteKey = 0
				}
			}
			noteVol := 0
			if (token & 0x40) == 0x40 { /* Volume Column.*/
				noteVol = int(buff[inOffset]&0x7F) + 0x10
				inOffset++
				if noteVol > 0x50 {
					noteVol = 0
				}
			}
			noteEffect := 0
			noteParam := 0
			if (token & 0x80) == 0x80 { /* Effect + Param.*/
				noteEffect = int(buff[inOffset])
				inOffset++
				noteParam = int(buff[inOffset])
				inOffset++
				if noteEffect < 1 || noteEffect >= 0x40 {
					noteEffect = 0
					noteParam = 0
				}
				if noteEffect > 0 {
					noteEffect += 0x80
				}
			}
			chanIdx := channelMap[token&0x1F]
			if chanIdx >= 0 {
				noteOffset := (rowIdx*m.numChannels + chanIdx) * 5
				pattern.data[noteOffset] = (byte)(noteKey)
				pattern.data[noteOffset+1] = (byte)(noteIns)
				pattern.data[noteOffset+2] = (byte)(noteVol)
				pattern.data[noteOffset+3] = (byte)(noteEffect)
				pattern.data[noteOffset+4] = (byte)(noteParam)
			}
		}
		moduleDataIdx += 2
	}
	m.defaultPanning = make([]int, m.numChannels)
	for chanIdx := 0; chanIdx < 32; chanIdx++ {
		if channelMap[chanIdx] < 0 {
			continue
		}
		panning := 7
		if stereoMode {
			panning = 12
			if buff[64+chanIdx] < 8 {
				panning = 3
			}
		}
		if defaultPan {
			panFlags := buff[moduleDataIdx+chanIdx]
			if (panFlags & 0x20) == 0x20 {
				panning = int(panFlags & 0xF)
			}
		}
		m.defaultPanning[channelMap[chanIdx]] = panning * 17
	}
	return m, nil
}

func DecodeMOD(reader *bufio.Reader) (*Module, error) {
	buff, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	m := NewModule()

	m.songName = string(buff[0:20])
	m.sequenceLength = int(buff[950] & 0x7F)
	m.restartPos = int(buff[951] & 0x7F)
	if m.restartPos >= m.sequenceLength {
		m.restartPos = 0
	}
	m.sequence = make([]int, 128)
	for seqIdx := 0; seqIdx < 128; seqIdx++ {
		patIdx := int(buff[952+seqIdx] & 0x7F)
		m.sequence[seqIdx] = patIdx
		if patIdx >= m.numPatterns {
			m.numPatterns = patIdx + 1
		}
	}
	switch binary.BigEndian.Uint16(buff[1082:]) {
	case 0x4b2e: /* M.K. */
		fallthrough
	case 0x4b21: /* M!K! */
		fallthrough
	case 0x5434: /* FLT4 */
		m.numChannels = 4
		m.c2Rate = PAL
		m.gain = 64
		break
	case 0x484e: /* xCHN */
		m.numChannels = int(int8(buff[1080])) - 48
		m.c2Rate = NTSC
		m.gain = 32
		break
	case 0x4348: /* xxCH */
		m.numChannels = (int(int8(buff[1080])) - 48) * 10
		m.numChannels += int(int8(buff[1081])) - 48
		m.c2Rate = NTSC
		m.gain = 32
		break
	default:
		panic("MOD Format not recognised!")
	}
	m.defaultGVol = 64
	m.defaultSpeed = 6
	m.defaultTempo = 125
	m.defaultPanning = make([]int, m.numChannels)
	for idx := 0; idx < m.numChannels; idx++ {
		m.defaultPanning[idx] = 51
		if (idx&3) == 1 || (idx&3) == 2 {
			m.defaultPanning[idx] = 204
		}
	}
	moduleDataIdx := 1084
	m.patterns = make([]*Pattern, m.numPatterns)

	for patIdx := 0; patIdx < m.numPatterns; patIdx++ {
		pattern := NewPattern(m.numChannels, 64)
		m.patterns[patIdx] = pattern
		for patDataIdx := 0; patDataIdx < len(pattern.data); patDataIdx += 5 {
			period := int(buff[moduleDataIdx]&0xF) << 8
			period = (period | int(buff[moduleDataIdx+1])) * 4
			if period > 112 {
				key, oct := 0, 0
				for period < 14510 {
					period *= 2
					oct++
				}
				for key < 12 {
					d1 := keyToPeriod[key] - period
					d2 := period - keyToPeriod[key+1]
					if d2 >= 0 {
						if d2 < d1 {
							key++
						}
						break
					}
					key++
				}
				pattern.data[patDataIdx] = (byte)(oct*12 + key)
			}
			ins := int(buff[moduleDataIdx+2]&0xF0) >> 4
			ins = ins | int(buff[moduleDataIdx]&0x10)
			pattern.data[patDataIdx+1] = (byte)(ins)
			effect := buff[moduleDataIdx+2] & 0x0F
			param := buff[moduleDataIdx+3] & 0xFF
			if param == 0 && (effect < 3 || effect == 0xA) {
				effect = 0
			}
			if param == 0 && (effect == 5 || effect == 6) {
				effect -= 2
			}
			if effect == 8 && m.numChannels == 4 {
				effect = 0
				param = 0
			}
			pattern.data[patDataIdx+3] = (byte)(effect)
			pattern.data[patDataIdx+4] = (byte)(param)
			moduleDataIdx += 4
		}
	}
	m.numInstruments = 31
	m.instruments = make([]*Instrument, m.numInstruments+1)
	m.instruments[0] = DefaultInstrument()
	for instIdx := 1; instIdx <= m.numInstruments; instIdx++ {
		instrument := DefaultInstrument()
		m.instruments[instIdx] = instrument
		sample := instrument.samples[0]
		instrument.name = string(buff[instIdx*30-10 : instIdx*30-10+22])
		sampleLength := int(binary.BigEndian.Uint16(buff[instIdx*30+12:])) * 2
		fineTune := int(buff[instIdx*30+14]&0xF) << 4
		sample.fineTune = fineTune - 256
		if fineTune < 128 {
			sample.fineTune = fineTune
		}
		volume := buff[instIdx*30+15] & 0x7F
		sample.volume = 64
		if volume <= 64 {
			sample.volume = int(volume)
		}
		sample.panning = -1
		sample.c2Rate = m.c2Rate
		loopStart := int(binary.BigEndian.Uint16(buff[instIdx*30+16:])) * 2
		loopLength := int(binary.BigEndian.Uint16(buff[instIdx*30+18:])) * 2
		sampleData := make([]int16, sampleLength)
		if moduleDataIdx+sampleLength > len(buff) {
			sampleLength = len(buff) - moduleDataIdx
		}
		if loopStart+loopLength > sampleLength {
			loopLength = sampleLength - loopStart
		}
		if loopLength < 4 {
			loopStart = sampleLength
			loopLength = 0
		}
		for idx, end := 0, sampleLength; idx < end; idx++ {
			sampleData[idx] = (int16)(int8(buff[moduleDataIdx])) << 8
			moduleDataIdx++
		}
		sample.setSampleData(sampleData, loopStart, loopLength, false)
	}
	return m, nil
}

func DecodeXM(reader *bufio.Reader) (*Module, error) {
	buff, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	if buff[58] != 0x04 && buff[59] != 0x01 {
		return nil, errors.New("XM format version must be 0x0104!")
	}

	m := NewModule()

	m.songName = string(buff[17:37])
	deltaEnv := bytes.Equal(buff[38:38+len(deltaEnvHeader)], deltaEnvHeader)
	dataOffset := 60 + binary.LittleEndian.Uint32(buff[60:])
	m.sequenceLength = int(binary.LittleEndian.Uint16(buff[64:]))
	m.restartPos = int(binary.LittleEndian.Uint16(buff[66:]))
	m.numChannels = int(binary.LittleEndian.Uint16(buff[68:]))
	m.numPatterns = int(binary.LittleEndian.Uint16(buff[70:]))
	m.numInstruments = int(binary.LittleEndian.Uint16(buff[72:]))
	m.linearPeriods = (binary.LittleEndian.Uint16(buff[74:]) & 0x1) > 0
	m.defaultGVol = 64
	m.defaultSpeed = int(binary.LittleEndian.Uint16(buff[76:]))
	m.defaultTempo = int(binary.LittleEndian.Uint16(buff[78:]))
	m.c2Rate = NTSC

	sequenceLength := m.sequenceLength
	numChannels := m.numChannels
	numPatterns := m.numPatterns
	numInstruments := m.numInstruments

	m.defaultPanning = make([]int, numChannels)
	m.sequence = make([]int, sequenceLength)
	m.patterns = make([]*Pattern, numPatterns)
	m.instruments = make([]*Instrument, numInstruments+1)

	defaultPanning := m.defaultPanning
	sequence := m.sequence
	patterns := m.patterns
	instruments := m.instruments

	for i := 0; i < numChannels; i++ {
		defaultPanning[i] = 128
	}
	for seqIdx := 0; seqIdx < sequenceLength; seqIdx++ {
		entry := buff[80+seqIdx]
		if int(entry) < numPatterns {
			sequence[seqIdx] = int(entry)
		}
	}

	for patIdx := 0; patIdx < numPatterns; patIdx++ {
		if buff[dataOffset+4] != 0 {
			return nil, errors.New("Unknown pattern packing type!")
		}
		numRows := int(binary.LittleEndian.Uint16(buff[dataOffset+5:]))
		numNotes := numRows * numChannels

		pattern := NewPattern(numChannels, numRows)
		patterns[patIdx] = pattern

		patternDataLength := uint32(binary.LittleEndian.Uint16(buff[dataOffset+7:]))
		dataOffset += binary.LittleEndian.Uint32(buff[dataOffset:])
		nextOffset := dataOffset + patternDataLength
		if patternDataLength > 0 {
			patternDataOffset := 0
			for note := 0; note < numNotes; note++ {
				flags := buff[dataOffset]
				if (flags & 0x80) == 0 {
					flags = 0x1F
				} else {
					dataOffset++
				}
				if (flags & 0x01) > 0 {
					pattern.data[patternDataOffset] = buff[dataOffset]
					dataOffset++
				}
				patternDataOffset++

				if (flags & 0x02) > 0 {
					pattern.data[patternDataOffset] = buff[dataOffset]
					dataOffset++
				}
				patternDataOffset++

				if (flags & 0x04) > 0 {
					pattern.data[patternDataOffset] = buff[dataOffset]
					dataOffset++
				}
				patternDataOffset++

				fxc, fxp := byte(0), byte(0)
				if (flags & 0x08) > 0 {
					fxc = buff[dataOffset]
					dataOffset++
				}
				if (flags & 0x10) > 0 {
					fxp = buff[dataOffset]
					dataOffset++
				}
				if fxc >= 0x40 {
					fxc = 0
					fxp = 0
				}
				pattern.data[patternDataOffset] = fxc
				patternDataOffset++
				pattern.data[patternDataOffset] = fxp
				patternDataOffset++
			}
		}
		dataOffset = nextOffset
	}

	instruments[0] = DefaultInstrument()
	for insIdx := 1; insIdx <= numInstruments; insIdx++ {
		instrument := &Instrument{}
		instruments[insIdx] = instrument
		instrument.name = string(buff[dataOffset+4 : dataOffset+4+22])
		numSamples := int(binary.LittleEndian.Uint16(buff[dataOffset+27:]))
		instrument.numSamples = numSamples

		if numSamples > 0 {
			instrument.samples = make([]*Sample, numSamples)
			for keyIdx := uint32(0); keyIdx < 96; keyIdx++ {
				instrument.keyToSample[keyIdx+1] = int(buff[dataOffset+33+keyIdx])
			}
			volEnv := &Envelope{}
			instrument.volumeEnvelope = volEnv
			volEnv.pointsTick = make([]int, 12)
			volEnv.pointsAmpl = make([]int, 12)

			pointTick := 0
			for point := uint32(0); point < 12; point++ {
				pointOffset := dataOffset + 129 + (point * 4)
				pt := int(binary.LittleEndian.Uint16(buff[pointOffset:]))
				volEnv.pointsTick[point] = pt
				if deltaEnv {
					volEnv.pointsTick[point] += pointTick
					pointTick = pt
				}

				volEnv.pointsAmpl[point] = int(binary.LittleEndian.Uint16(buff[pointOffset+2:]))
			}

			panEnv := &Envelope{}
			instrument.panningEnvelope = panEnv
			panEnv.pointsTick = make([]int, 12)
			panEnv.pointsAmpl = make([]int, 12)
			pointTick = 0
			for point := uint32(0); point < 12; point++ {
				pointOffset := dataOffset + 177 + (point * 4)
				pt := int(binary.LittleEndian.Uint16(buff[pointOffset:]))
				panEnv.pointsTick[point] = pt
				if deltaEnv {
					panEnv.pointsTick[point] += pointTick
					pointTick = pt
				}

				panEnv.pointsAmpl[point] = int(binary.LittleEndian.Uint16(buff[pointOffset+2:]))
			}

			volEnv.numPoints = int(buff[dataOffset+225])
			if volEnv.numPoints > 12 {
				volEnv.numPoints = 0
			}
			panEnv.numPoints = int(buff[dataOffset+226])
			if panEnv.numPoints > 12 {
				panEnv.numPoints = 0
			}
			volEnv.sustainTick = volEnv.pointsTick[buff[dataOffset+227]]
			volEnv.loopStartTick = volEnv.pointsTick[buff[dataOffset+228]]
			volEnv.loopEndTick = volEnv.pointsTick[buff[dataOffset+229]]
			panEnv.sustainTick = panEnv.pointsTick[buff[dataOffset+230]]
			panEnv.loopStartTick = panEnv.pointsTick[buff[dataOffset+231]]
			panEnv.loopEndTick = panEnv.pointsTick[buff[dataOffset+232]]
			volEnv.enabled = volEnv.numPoints > 0 && (buff[dataOffset+233]&0x1) > 0
			volEnv.sustain = (buff[dataOffset+233] & 0x2) > 0
			volEnv.looped = (buff[dataOffset+233] & 0x4) > 0
			panEnv.enabled = panEnv.numPoints > 0 && (buff[dataOffset+234]&0x1) > 0
			panEnv.sustain = (buff[dataOffset+234] & 0x2) > 0
			panEnv.looped = (buff[dataOffset+234] & 0x4) > 0
			instrument.vibratoType = int(buff[dataOffset+235])
			instrument.vibratoSweep = int(buff[dataOffset+236])
			instrument.vibratoDepth = int(buff[dataOffset+237])
			instrument.vibratoRate = int(buff[dataOffset+238])
			instrument.volumeFadeOut = int(binary.LittleEndian.Uint16((buff[dataOffset+239:])))
		}

		dataOffset += binary.LittleEndian.Uint32(buff[dataOffset:])

		sampleHeaderOffset := dataOffset
		dataOffset += uint32(numSamples) * 40
		for samIdx := 0; samIdx < numSamples; samIdx++ {
			sample := &Sample{}
			instrument.samples[samIdx] = sample

			sampleDataBytes := binary.LittleEndian.Uint32(buff[sampleHeaderOffset:])
			sampleLoopStart := binary.LittleEndian.Uint32(buff[sampleHeaderOffset+4:])
			sampleLoopLength := binary.LittleEndian.Uint32(buff[sampleHeaderOffset+8:])

			sample.volume = int(int8(buff[sampleHeaderOffset+12]))
			sample.fineTune = int(int8(buff[sampleHeaderOffset+13]))
			sample.c2Rate = NTSC
			looped := (buff[sampleHeaderOffset+14] & 0x3) > 0
			pingPong := (buff[sampleHeaderOffset+14] & 0x2) > 0
			sixteenBit := (buff[sampleHeaderOffset+14] & 0x10) > 0
			sample.panning = int(buff[sampleHeaderOffset+15])
			sample.relNote = int(int8(buff[sampleHeaderOffset+16]))
			sample.name = string(buff[sampleHeaderOffset+18 : sampleHeaderOffset+18+22])
			sampleHeaderOffset += 40
			sampleDataLength := sampleDataBytes
			if sixteenBit {
				sampleDataLength /= 2
				sampleLoopStart /= 2
				sampleLoopLength /= 2
			}
			if !looped || (sampleLoopStart+sampleLoopLength) > sampleDataLength {
				sampleLoopStart = sampleDataLength
				sampleLoopLength = 0
			}
			sampleData := make([]int16, sampleDataLength)
			if sixteenBit {
				ampl := uint16(0)
				for outIdx := uint32(0); outIdx < sampleDataLength; outIdx++ {
					inIdx := dataOffset + outIdx*2
					ampl += uint16(buff[inIdx])
					ampl += uint16(buff[inIdx+1]) << 8
					sampleData[outIdx] = int16(ampl)
				}
			} else {
				ampl := byte(0)
				for outIdx := uint32(0); outIdx < sampleDataLength; outIdx++ {
					ampl += buff[dataOffset+outIdx]
					sampleData[outIdx] = int16(uint16(ampl) << 8)
				}
			}

			sample.setSampleData(sampleData, int(sampleLoopStart), int(sampleLoopLength), pingPong)
			dataOffset += sampleDataBytes
		}
	}

	return m, nil
}
