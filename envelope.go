package ibxmgo

type Envelope struct {
	numPoints                               int
	pointsTick                              []int
	pointsAmpl                              []int
	enabled, sustain, looped                bool
	sustainTick, loopStartTick, loopEndTick int
}

func DefaultEnvelope() *Envelope {
	return &Envelope{
		numPoints:  1,
		pointsTick: []int{1},
		pointsAmpl: []int{1},
	}
}

func (this *Envelope) nextTick(tick int, keyOn bool) int {
	tick++
	if this.looped && tick >= this.loopEndTick {
		tick = this.loopStartTick
	}
	if this.sustain && keyOn && tick >= this.sustainTick {
		tick = this.sustainTick
	}
	return tick
}

func (this *Envelope) calculateAmpl(tick int) int {
	ampl := this.pointsAmpl[this.numPoints-1]
	if tick < this.pointsTick[this.numPoints-1] {
		point := 0
		for idx := 1; idx < this.numPoints; idx++ {
			if this.pointsTick[idx] <= tick {
				point = idx
			}
		}
		dt := this.pointsTick[point+1] - this.pointsTick[point]
		da := this.pointsAmpl[point+1] - this.pointsAmpl[point]
		ampl = this.pointsAmpl[point]
		ampl += ((da << 24) / dt) * (tick - this.pointsTick[point]) >> 24
	}
	return ampl
}
