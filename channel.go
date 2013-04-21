package ibxmgo

type Interpolation int

const (
	NEAREST = Interpolation(0)
	LINEAR  = Interpolation(1)
	SINC    = Interpolation(2)
)

var (
	periodTable = []int{
		// Periods for keys -11 to 1 with 8 finetune values.
		54784, 54390, 53999, 53610, 53224, 52841, 52461, 52084,
		51709, 51337, 50968, 50601, 50237, 49876, 49517, 49161,
		48807, 48456, 48107, 47761, 47418, 47076, 46738, 46401,
		46068, 45736, 45407, 45081, 44756, 44434, 44115, 43797,
		43482, 43169, 42859, 42550, 42244, 41940, 41639, 41339,
		41042, 40746, 40453, 40162, 39873, 39586, 39302, 39019,
		38738, 38459, 38183, 37908, 37635, 37365, 37096, 36829,
		36564, 36301, 36040, 35780, 35523, 35267, 35014, 34762,
		34512, 34263, 34017, 33772, 33529, 33288, 33049, 32811,
		32575, 32340, 32108, 31877, 31647, 31420, 31194, 30969,
		30746, 30525, 30306, 30088, 29871, 29656, 29443, 29231,
		29021, 28812, 28605, 28399, 28195, 27992, 27790, 27590,
		27392, 27195, 26999, 26805, 26612, 26421, 26231, 26042,
	}
	freqTable = []int{
		// Frequency for keys 109 to 121 with 8 fractional values.
		267616, 269555, 271509, 273476, 275458, 277454, 279464, 281489,
		283529, 285584, 287653, 289738, 291837, 293952, 296082, 298228,
		300389, 302566, 304758, 306966, 309191, 311431, 313688, 315961,
		318251, 320557, 322880, 325220, 327576, 329950, 332341, 334749,
		337175, 339618, 342079, 344558, 347055, 349570, 352103, 354655,
		357225, 359813, 362420, 365047, 367692, 370356, 373040, 375743,
		378466, 381209, 383971, 386754, 389556, 392379, 395222, 398086,
		400971, 403877, 406803, 409751, 412720, 415711, 418723, 421758,
		424814, 427892, 430993, 434116, 437262, 440430, 443622, 446837,
		450075, 453336, 456621, 459930, 463263, 466620, 470001, 473407,
		476838, 480293, 483773, 487279, 490810, 494367, 497949, 501557,
		505192, 508853, 512540, 516254, 519995, 523763, 527558, 531381,
		535232, 539111, 543017, 546952, 550915, 554908, 558929, 562979,
	}

	arpTuning = []int16{
		4096, 4340, 4598, 4871, 5161, 5468, 5793, 6137,
		6502, 6889, 7298, 7732, 8192, 8679, 9195, 9742,
	}

	sineTable = []int16{
		0, 24, 49, 74, 97, 120, 141, 161, 180, 197, 212, 224, 235, 244, 250, 253,
		255, 253, 250, 244, 235, 224, 212, 197, 180, 161, 141, 120, 97, 74, 49, 24,
	}
)

type Channel struct {
	module     *Module
	globalVol  *int
	instrument *Instrument
	sample     *Sample
	keyOn      bool
	noteKey, noteIns, noteVol, noteEffect, noteParam,
	sampleIdx, sampleFra, freq, ampl, pann,
	volume, panning, fadeOutVol, volEnvTick, panEnvTick,
	period, portaPeriod, retrigCount, fxCount, autoVibratoCount,
	portaUpParam, portaDownParam, tonePortaParam, offsetParam,
	finePortaUpParam, finePortaDownParam, extraFinePortaParam,
	arpeggioParam, vslideParam, globalVslideParam, panningSlideParam,
	fineVslideUpParam, fineVslideDownParam,
	retrigVolume, retrigTicks, tremorOnTicks, tremorOffTicks,
	vibratoType, vibratoPhase, vibratoSpeed, vibratoDepth,
	tremoloType, tremoloPhase, tremoloSpeed, tremoloDepth,
	tremoloAdd, vibratoAdd, arpeggioAdd,
	id, randomSeed,
	plRow int
}

func NewChannel(module *Module, id int, globalVol *int) *Channel {
	this := &Channel{}
	this.module = module
	this.id = id
	this.globalVol = globalVol
	this.panning = module.defaultPanning[id]
	this.instrument = DefaultInstrument()
	this.sample = this.instrument.samples[0]
	this.randomSeed = (id + 1) * 0xABCDEF
	return this
}

func (this *Channel) resample(outBuf []int32, offset, length, sampleRate int, interpolation Interpolation) {
	if this.ampl <= 0 {
		return
	}
	lAmpl := this.ampl * (255 - this.pann) >> 8
	rAmpl := this.ampl * this.pann >> 8
	step := (this.freq << (FP_SHIFT - 3)) / (sampleRate >> 3)
	switch interpolation {
	case NEAREST:
		//this.sample.resampleNearest(this.sampleIdx, this.sampleFra, step, lAmpl, rAmpl, outBuf, offset, length)
		break
	case LINEAR:
		this.sample.resampleLinear(this.sampleIdx, this.sampleFra, step, lAmpl, rAmpl, outBuf, offset, length)
	default:
		this.sample.resampleLinear(this.sampleIdx, this.sampleFra, step, lAmpl, rAmpl, outBuf, offset, length)
	case SINC:
		this.sample.resampleSinc(this.sampleIdx, this.sampleFra, step, lAmpl, rAmpl, outBuf, offset, length)
		break
	}
}

func (this *Channel) updateSampleIdx(length int, sampleRate int) {
	step := (this.freq << (FP_SHIFT - 3)) / (sampleRate >> 3)
	this.sampleFra += step * length
	this.sampleIdx = this.sample.normaliseSampleIdx(this.sampleIdx + (this.sampleFra >> FP_SHIFT))
	this.sampleFra &= FP_MASK
}

func (this *Channel) tick() {
	this.vibratoAdd = 0
	this.fxCount++
	this.retrigCount++
	if !(this.noteEffect == 0x7D && this.fxCount <= this.noteParam) {
		switch this.noteVol & 0xF0 {
		case 0x60: /* Vol Slide Down.*/
			this.volume -= this.noteVol & 0xF
			if this.volume < 0 {
				this.volume = 0
			}
			break
		case 0x70: /* Vol Slide Up.*/
			this.volume += this.noteVol & 0xF
			if this.volume > 64 {
				this.volume = 64
			}
			break
		case 0xB0: /* Vibrato.*/
			this.vibratoPhase += this.vibratoSpeed
			//vibrato( false );
			break
		case 0xD0: /* Pan Slide Left.*/
			this.panning -= this.noteVol & 0xF
			if this.panning < 0 {
				this.panning = 0
			}
			break
		case 0xE0: /* Pan Slide Right.*/
			this.panning += this.noteVol & 0xF
			if this.panning > 255 {
				this.panning = 255
			}
			break
		case 0xF0: /* Tone Porta.*/
			this.tonePortamento()
			break
		}
	}
	switch this.noteEffect {
	case 0x01:
		fallthrough
	case 0x86: /* Porta Up. */
		this.portamentoUp(this.portaUpParam)
		break
	case 0x02:
		fallthrough
	case 0x85: /* Porta Down. */
		this.portamentoDown(this.portaDownParam)
		break
	case 0x03:
		fallthrough
	case 0x87: /* Tone Porta. */
		this.tonePortamento()
		break
	case 0x04:
		fallthrough
	case 0x88: /* Vibrato. */
		this.vibratoPhase += this.vibratoSpeed
		this.vibrato(false)
		break
	case 0x05:
		fallthrough
	case 0x8C: /* Tone Porta + Vol Slide. */
		this.tonePortamento()
		this.volumeSlide()
		break
	case 0x06:
		fallthrough
	case 0x8B: /* Vibrato + Vol Slide. */
		this.vibratoPhase += this.vibratoSpeed
		this.vibrato(false)
		this.volumeSlide()
		break
	case 0x07:
		fallthrough
	case 0x92: /* Tremolo. */
		this.tremoloPhase += this.tremoloSpeed
		this.tremolo()
		break
	case 0x0A:
		fallthrough
	case 0x84: /* Vol Slide. */
		this.volumeSlide()
		break
	case 0x11: /* Global Volume Slide. */
		*this.globalVol += (this.globalVslideParam >> 4) - (this.globalVslideParam & 0xF)
		if *this.globalVol < 0 {
			*this.globalVol = 0
		}
		if *this.globalVol > 64 {
			*this.globalVol = 64
		}
		break
	case 0x19: /* Panning Slide. */
		this.panning += (this.panningSlideParam >> 4) - (this.panningSlideParam & 0xF)
		if this.panning < 0 {
			this.panning = 0
		}
		if this.panning > 255 {
			this.panning = 255
		}
		break
	case 0x1B:
		fallthrough
	case 0x91: /* Retrig + Vol Slide. */
		this.retrigVolSlide()
		break
	case 0x1D:
		fallthrough
	case 0x89: /* Tremor. */
		this.tremor()
		break
	case 0x79: /* Retrig. */
		if this.fxCount >= this.noteParam {
			this.fxCount = 0
			this.sampleIdx = 0
			this.sampleFra = 0
		}
		break
	case 0x7C:
		fallthrough
	case 0xFC: /* Note Cut. */
		if this.noteParam == this.fxCount {
			this.volume = 0
		}
		break
	case 0x7D:
		fallthrough
	case 0xFD: /* Note Delay. */
		if this.noteParam == this.fxCount {
			this.trigger()
		}
		break
	case 0x8A: /* Arpeggio. */
		if this.fxCount > 2 {
			this.fxCount = 0
		}
		if this.fxCount == 0 {
			this.arpeggioAdd = 0
		}
		if this.fxCount == 1 {
			this.arpeggioAdd = this.arpeggioParam >> 4
		}
		if this.fxCount == 2 {
			this.arpeggioAdd = this.arpeggioParam & 0xF
		}
		break
	case 0x95: /* Fine Vibrato. */
		this.vibratoPhase += this.vibratoSpeed
		this.vibrato(true)
		break
	}
	this.autoVibrato()
	this.calculateFrequency()
	this.calculateAmplitude()
	this.updateEnvelopes()
}

func (this *Channel) row(note *Note) {
	this.noteKey = note.key
	this.noteIns = note.instrument
	this.noteVol = note.volume
	this.noteEffect = note.effect
	this.noteParam = note.param
	this.retrigCount++
	this.vibratoAdd = 0
	this.tremoloAdd = 0
	this.arpeggioAdd = 0
	this.fxCount = 0
	if this.noteEffect != 0x7D && this.noteEffect != 0xFD {
		this.trigger()
	}
	switch this.noteEffect {
	case 0x01:
		fallthrough
	case 0x86: /* Porta Up. */
		if this.noteParam > 0 {
			this.portaUpParam = this.noteParam
		}
		this.portamentoUp(this.portaUpParam)
		break
	case 0x02:
		fallthrough
	case 0x85: /* Porta Down. */
		if this.noteParam > 0 {
			this.portaDownParam = this.noteParam
		}
		this.portamentoDown(this.portaDownParam)
		break
	case 0x03:
		fallthrough
	case 0x87: /* Tone Porta. */
		if this.noteParam > 0 {
			this.tonePortaParam = this.noteParam
		}
		break
	case 0x04:
		fallthrough
	case 0x88: /* Vibrato. */
		if (this.noteParam >> 4) > 0 {
			this.vibratoSpeed = this.noteParam >> 4
		}
		if (this.noteParam & 0xF) > 0 {
			this.vibratoDepth = this.noteParam & 0xF
		}
		this.vibrato(false)
		break
	case 0x05:
		fallthrough
	case 0x8C: /* Tone Porta + Vol Slide. */
		if this.noteParam > 0 {
			this.vslideParam = this.noteParam
		}
		this.volumeSlide()
		break
	case 0x06:
		fallthrough
	case 0x8B: /* Vibrato + Vol Slide. */
		if this.noteParam > 0 {
			this.vslideParam = this.noteParam
		}
		this.vibrato(false)
		this.volumeSlide()
		break
	case 0x07:
		fallthrough
	case 0x92: /* Tremolo. */
		if (this.noteParam >> 4) > 0 {
			this.tremoloSpeed = this.noteParam >> 4
		}
		if (this.noteParam & 0xF) > 0 {
			this.tremoloDepth = this.noteParam & 0xF
		}
		this.tremolo()
		break
	case 0x08: /* Set Panning.*/
		this.panning = this.noteParam & 0xFF
		break
	case 0x09:
		fallthrough
	case 0x8F: /* Set Sample Offset. */
		if this.noteParam > 0 {
			this.offsetParam = this.noteParam
		}
		this.sampleIdx = this.offsetParam << 8
		this.sampleFra = 0
		break
	case 0x0A:
		fallthrough
	case 0x84: /* Vol Slide. */
		if this.noteParam > 0 {
			this.vslideParam = this.noteParam
		}
		this.volumeSlide()
		break
	case 0x0C: /* Set Volume. */
		this.volume = this.noteParam & 63
		if this.noteParam >= 64 {
			this.volume = 64
		}
		break
	case 0x10:
		fallthrough
	case 0x96: /* Set Global Volume. */
		*this.globalVol = this.noteParam & 63
		if this.noteParam >= 64 {
			*this.globalVol = 64
		}
		break
	case 0x11: /* Global Volume Slide. */
		if this.noteParam > 0 {
			this.globalVslideParam = this.noteParam
		}
		break
	case 0x14: /* Key Off. */
		this.keyOn = false
		break
	case 0x15: /* Set Envelope Tick. */
		this.volEnvTick = this.noteParam & 0xFF
		this.panEnvTick = this.volEnvTick
		break
	case 0x19: /* Panning Slide. */
		if this.noteParam > 0 {
			this.panningSlideParam = this.noteParam
		}
		break
	case 0x1B:
		fallthrough
	case 0x91: /* Retrig + Vol Slide. */
		if (this.noteParam >> 4) > 0 {
			this.retrigVolume = this.noteParam >> 4
		}
		if (this.noteParam & 0xF) > 0 {
			this.retrigTicks = this.noteParam & 0xF
		}
		this.retrigVolSlide()
		break
	case 0x1D:
		fallthrough
	case 0x89: /* Tremor. */
		if (this.noteParam >> 4) > 0 {
			this.tremorOnTicks = this.noteParam >> 4
		}
		if (this.noteParam & 0xF) > 0 {
			this.tremorOffTicks = this.noteParam & 0xF
		}
		this.tremor()
		break
	case 0x21: /* Extra Fine Porta. */
		if this.noteParam > 0 {
			this.extraFinePortaParam = this.noteParam
		}
		switch this.extraFinePortaParam & 0xF0 {
		case 0x10:
			this.portamentoUp(0xE0 | (this.extraFinePortaParam & 0xF))
			break
		case 0x20:
			this.portamentoDown(0xE0 | (this.extraFinePortaParam & 0xF))
			break
		}
		break
	case 0x71: /* Fine Porta Up. */
		if this.noteParam > 0 {
			this.finePortaUpParam = this.noteParam
		}
		this.portamentoUp(0xF0 | (this.finePortaUpParam & 0xF))
		break
	case 0x72: /* Fine Porta Down. */
		if this.noteParam > 0 {
			this.finePortaDownParam = this.noteParam
		}
		this.portamentoDown(0xF0 | (this.finePortaDownParam & 0xF))
		break
	case 0x74:
		fallthrough
	case 0xF3: /* Set Vibrato Waveform. */
		if this.noteParam < 8 {
			this.vibratoType = this.noteParam
		}
		break
	case 0x77:
		fallthrough
	case 0xF4: /* Set Tremolo Waveform. */
		if this.noteParam < 8 {
			this.tremoloType = this.noteParam
		}
		break
	case 0x7A: /* Fine Vol Slide Up. */
		if this.noteParam > 0 {
			this.fineVslideUpParam = this.noteParam
		}
		this.volume += this.fineVslideUpParam
		if this.volume > 64 {
			this.volume = 64
		}
		break
	case 0x7B: /* Fine Vol Slide Down. */
		if this.noteParam > 0 {
			this.fineVslideDownParam = this.noteParam
		}
		this.volume -= this.fineVslideDownParam
		if this.volume < 0 {
			this.volume = 0
		}
		break
	case 0x7C:
		fallthrough
	case 0xFC: /* Note Cut. */
		if this.noteParam <= 0 {
			this.volume = 0
		}
		break
	case 0x7D:
		fallthrough
	case 0xFD: /* Note Delay. */
		if this.noteParam <= 0 {
			this.trigger()
		}
		break
	case 0x8A: /* Arpeggio. */
		if this.noteParam > 0 {
			this.arpeggioParam = this.noteParam
		}
		break
	case 0x95: /* Fine Vibrato.*/
		if (this.noteParam >> 4) > 0 {
			this.vibratoSpeed = this.noteParam >> 4
		}
		if (this.noteParam & 0xF) > 0 {
			this.vibratoDepth = this.noteParam & 0xF
		}
		this.vibrato(true)
		break
	case 0xF8: /* Set Panning. */
		this.panning = this.noteParam * 17
		break
	}
	this.autoVibrato()
	this.calculateFrequency()
	this.calculateAmplitude()
	this.updateEnvelopes()
}

func (this *Channel) updateEnvelopes() {
	if this.instrument.volumeEnvelope.enabled {
		if !this.keyOn {
			this.fadeOutVol -= this.instrument.volumeFadeOut
			if this.fadeOutVol < 0 {
				this.fadeOutVol = 0
			}
		}
		this.volEnvTick = this.instrument.volumeEnvelope.nextTick(this.volEnvTick, this.keyOn)
	}
	if this.instrument.panningEnvelope.enabled {
		this.panEnvTick = this.instrument.panningEnvelope.nextTick(this.panEnvTick, this.keyOn)
	}
}

func (this *Channel) autoVibrato() {
	depth := this.instrument.vibratoDepth & 0x7F
	if depth > 0 {
		sweep := this.instrument.vibratoSweep & 0x7F
		rate := this.instrument.vibratoRate & 0x7F
		typ := this.instrument.vibratoType
		if this.autoVibratoCount < sweep {
			depth = depth * this.autoVibratoCount / sweep
		}
		this.vibratoAdd += this.waveform(this.autoVibratoCount*rate>>2, typ+4) * depth >> 8
		this.autoVibratoCount++
	}
}

func (this *Channel) volumeSlide() {
	up := this.vslideParam >> 4
	down := this.vslideParam & 0xF
	if down == 0xF && up > 0 { /* Fine slide up.*/
		if this.fxCount == 0 {
			this.volume += up
		}
	} else if up == 0xF && down > 0 { /* Fine slide down.*/
		if this.fxCount == 0 {
			this.volume -= down
		}
	} else if this.fxCount > 0 || this.module.fastVolSlides { /* Normal.*/
		this.volume += up - down
	}
	if this.volume > 64 {
		this.volume = 64
	}
	if this.volume < 0 {
		this.volume = 0
	}
}

func (this *Channel) portamentoUp(param int) {
	switch param & 0xF0 {
	case 0xE0: /* Extra-fine porta.*/
		if this.fxCount == 0 {
			this.period -= param & 0xF
		}
		break
	case 0xF0: /* Fine porta.*/
		if this.fxCount == 0 {
			this.period -= (param & 0xF) << 2
		}
		break
	default: /* Normal porta.*/
		if this.fxCount > 0 {
			this.period -= param << 2
		}
		break
	}
	if this.period < 0 {
		this.period = 0
	}
}

func (this *Channel) portamentoDown(param int) {
	if this.period > 0 {
		switch param & 0xF0 {
		case 0xE0: /* Extra-fine porta.*/
			if this.fxCount == 0 {
				this.period += param & 0xF
			}
			break
		case 0xF0: /* Fine porta.*/
			if this.fxCount == 0 {
				this.period += (param & 0xF) << 2
			}
			break
		default: /* Normal porta.*/
			if this.fxCount > 0 {
				this.period += param << 2
			}
			break
		}
		if this.period > 65535 {
			this.period = 65535
		}
	}
}

func (this *Channel) tonePortamento() {
	if this.period > 0 {
		if this.period < this.portaPeriod {
			this.period += this.tonePortaParam << 2
			if this.period > this.portaPeriod {
				this.period = this.portaPeriod
			}
		} else {
			this.period -= this.tonePortaParam << 2
			if this.period < this.portaPeriod {
				this.period = this.portaPeriod
			}
		}
	}
}

func (this *Channel) vibrato(fine bool) {
	this.vibratoAdd = this.waveform(this.vibratoPhase, this.vibratoType&0x3) * this.vibratoDepth
	if fine {
		this.vibratoAdd = this.vibratoAdd >> 7
	} else {
		this.vibratoAdd = this.vibratoAdd >> 5
	}
}

func (this *Channel) tremolo() {
	this.tremoloAdd = this.waveform(this.tremoloPhase, this.tremoloType&0x3) * this.tremoloDepth >> 6
}

func (this *Channel) waveform(phase int, typ int) int {
	amplitude := 0
	switch typ {
	default: /* Sine. */
		amplitude = int(sineTable[phase&0x1F])
		if (phase & 0x20) > 0 {
			amplitude = -amplitude
		}
		break
	case 6: /* Saw Up.*/
		amplitude = (((phase + 0x20) & 0x3F) << 3) - 255
		break
	case 1:
		fallthrough
	case 7: /* Saw Down. */
		amplitude = 255 - (((phase + 0x20) & 0x3F) << 3)
		break
	case 2:
		fallthrough
	case 5: /* Square. */
		if (phase & 0x20) > 0 {
			amplitude = 255
		} else {
			amplitude = -255
		}
		break
	case 3:
		fallthrough
	case 8: /* Random. */
		amplitude = (this.randomSeed >> 20) - 255
		this.randomSeed = (this.randomSeed*65 + 17) & 0x1FFFFFFF
		break
	}
	return amplitude
}

func (this *Channel) tremor() {
	if this.retrigCount >= this.tremorOnTicks {
		this.tremoloAdd = -64
	}
	if this.retrigCount >= (this.tremorOnTicks + this.tremorOffTicks) {
		this.tremoloAdd = 0
		this.retrigCount = 0
	}
}

func (this *Channel) retrigVolSlide() {
	if this.retrigCount >= this.retrigTicks {
		this.retrigCount = 0
		this.sampleIdx = 0
		this.sampleFra = 0
		switch this.retrigVolume {
		case 0x1:
			this.volume -= 1
			break
		case 0x2:
			this.volume -= 2
			break
		case 0x3:
			this.volume -= 4
			break
		case 0x4:
			this.volume -= 8
			break
		case 0x5:
			this.volume -= 16
			break
		case 0x6:
			this.volume -= this.volume / 3
			break
		case 0x7:
			this.volume >>= 1
			break
		case 0x8: /* ? */ break
		case 0x9:
			this.volume += 1
			break
		case 0xA:
			this.volume += 2
			break
		case 0xB:
			this.volume += 4
			break
		case 0xC:
			this.volume += 8
			break
		case 0xD:
			this.volume += 16
			break
		case 0xE:
			this.volume += this.volume >> 1
			break
		case 0xF:
			this.volume <<= 1
			break
		}
		if this.volume < 0 {
			this.volume = 0
		}
		if this.volume > 64 {
			this.volume = 64
		}
	}
}

func (this *Channel) calculateFrequency() {
	if this.module.linearPeriods {
		per := this.period + this.vibratoAdd - (this.arpeggioAdd << 6)
		if per < 28 || per > 7680 {
			per = 7680
		}
		tone := 7680 - per
		i := (tone >> 3) % 96
		c := freqTable[i]
		m := freqTable[i+1] - c
		x := tone & 0x7
		y := ((m * x) >> 3) + c
		this.freq = y >> uint(9-tone/768)
	} else {
		per := this.period + this.vibratoAdd
		if per < 28 {
			per = periodTable[0]
		}
		this.freq = int(this.module.c2Rate) * 1712 / per
		this.freq = (this.freq * int(arpTuning[this.arpeggioAdd]) >> 12) & 0x7FFFF
	}
}

func (this *Channel) calculateAmplitude() {
	envVol := 0
	if this.keyOn {
		envVol = 64
	}
	if this.instrument.volumeEnvelope.enabled {
		envVol = this.instrument.volumeEnvelope.calculateAmpl(this.volEnvTick)
	}
	vol := this.volume + this.tremoloAdd
	if vol > 64 {
		vol = 64
	}
	if vol < 0 {
		vol = 0
	}
	vol = (vol * this.module.gain * FP_ONE) >> 13
	vol = (vol * this.fadeOutVol) >> 15
	this.ampl = (vol * *this.globalVol * envVol) >> 12

	envPan := 32
	if this.instrument.panningEnvelope.enabled {
		envPan = this.instrument.panningEnvelope.calculateAmpl(this.panEnvTick)
	}
	panRange := (255 - this.panning)
	if this.panning < 128 {
		panRange = this.panning
	}
	this.pann = this.panning + (panRange * (envPan - 32) >> 5)
}

func (this *Channel) trigger() {
	if this.noteIns > 0 && this.noteIns <= this.module.numInstruments {
		this.instrument = this.module.instruments[this.noteIns]
		k := 0
		if this.noteKey < 97 {
			k = this.noteKey
		}
		sam := this.instrument.samples[this.instrument.keyToSample[k]]
		this.volume = sam.volume & 0x3F
		if sam.volume >= 64 {
			this.volume = 64
		}
		if sam.panning >= 0 {
			this.panning = sam.panning & 0xFF
		}
		if this.period > 0 && sam.looped() {
			this.sample = sam
		} /* Amiga trigger.*/
		this.volEnvTick = 0
		this.panEnvTick = 0
		this.fadeOutVol = 32768
		this.keyOn = true
	}
	if this.noteVol >= 0x10 && this.noteVol < 0x60 {
		this.volume = 64
		if this.noteVol < 0x50 {
			this.volume = this.noteVol - 0x10
		}
	}
	switch this.noteVol & 0xF0 {
	case 0x80: /* Fine Vol Down.*/
		this.volume -= this.noteVol & 0xF
		if this.volume < 0 {
			this.volume = 0
		}
		break
	case 0x90: /* Fine Vol Up.*/
		this.volume += this.noteVol & 0xF
		if this.volume > 64 {
			this.volume = 64
		}
		break
	case 0xA0: /* Set Vibrato Speed.*/
		if (this.noteVol & 0xF) > 0 {
			this.vibratoSpeed = this.noteVol & 0xF
		}
		break
	case 0xB0: /* Vibrato.*/
		if (this.noteVol & 0xF) > 0 {
			this.vibratoDepth = this.noteVol & 0xF
		}
		this.vibrato(false)
		break
	case 0xC0: /* Set Panning.*/
		this.panning = (this.noteVol & 0xF) * 17
		break
	case 0xF0: /* Tone Porta.*/
		if (this.noteVol & 0xF) > 0 {
			this.tonePortaParam = this.noteVol & 0xF
		}
		break
	}
	if this.noteKey > 0 {
		if this.noteKey > 96 {
			this.keyOn = false
		} else {
			isPorta := (this.noteVol&0xF0) == 0xF0 ||
				this.noteEffect == 0x03 || this.noteEffect == 0x05 ||
				this.noteEffect == 0x87 || this.noteEffect == 0x8C
			if !isPorta {
				this.sample = this.instrument.samples[this.instrument.keyToSample[this.noteKey]]
			}
			fineTune := this.sample.fineTune
			if this.noteEffect == 0x75 || this.noteEffect == 0xF2 { /* Set FineTune. */
				fineTune = (int)(int8(((this.noteParam & 0xF) << 4)))
			}
			key := this.noteKey + this.sample.relNote
			if key < 1 {
				key = 1
			}
			if key > 120 {
				key = 120
			}
			if this.module.linearPeriods {
				this.portaPeriod = 7680 - ((key - 1) << 6) - int(fineTune>>1)
			} else {
				tone := 768 + ((key - 1) << 6) + int(fineTune>>1)
				i := (tone >> 3) % 96
				c := periodTable[i]
				m := periodTable[i+1] - c
				x := tone & 0x7
				y := ((m * x) >> 3) + c
				this.portaPeriod = y >> uint(tone/768)
				this.portaPeriod = int(this.module.c2Rate) * this.portaPeriod / int(this.sample.c2Rate)
			}
			if !isPorta {
				this.period = this.portaPeriod
				this.sampleIdx = 0
				this.sampleFra = 0
				if this.vibratoType < 4 {
					this.vibratoPhase = 0
				}
				if this.tremoloType < 4 {
					this.tremoloPhase = 0
				}
				this.retrigCount = 0
				this.autoVibratoCount = 0
			}
		}
	}
}
