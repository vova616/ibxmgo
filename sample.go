package ibxmgo

var (
	SINC_TABLE = []int16{
		0, 0, 0, 0, 0, 0, 0, 32767, 0, 0, 0, 0, 0, 0, 0, 0,
		-1, 7, -31, 103, -279, 671, -1731, 32546, 2006, -747, 312, -118, 37, -8, 1, 0,
		-1, 12, -56, 190, -516, 1246, -3167, 31887, 4259, -1549, 648, -248, 78, -18, 2, 0,
		-1, 15, -74, 257, -707, 1714, -4299, 30808, 6722, -2375, 994, -384, 122, -29, 4, 0,
		-2, 17, -87, 305, -849, 2067, -5127, 29336, 9351, -3196, 1338, -520, 169, -41, 6, 0,
		-2, 18, -93, 334, -941, 2303, -5659, 27510, 12092, -3974, 1662, -652, 214, -53, 8, 0,
		-1, 17, -95, 346, -985, 2425, -5912, 25375, 14888, -4673, 1951, -771, 257, -65, 10, 0,
		-1, 16, -92, 341, -985, 2439, -5908, 22985, 17679, -5254, 2188, -871, 294, -76, 13, -1,
		-1, 15, -85, 323, -945, 2355, -5678, 20399, 20399, -5678, 2355, -945, 323, -85, 15, -1,
		-1, 13, -76, 294, -871, 2188, -5254, 17679, 22985, -5908, 2439, -985, 341, -92, 16, -1,
		0, 10, -65, 257, -771, 1951, -4673, 14888, 25375, -5912, 2425, -985, 346, -95, 17, -1,
		0, 8, -53, 214, -652, 1662, -3974, 12092, 27510, -5659, 2303, -941, 334, -93, 18, -2,
		0, 6, -41, 169, -520, 1338, -3196, 9351, 29336, -5127, 2067, -849, 305, -87, 17, -2,
		0, 4, -29, 122, -384, 994, -2375, 6722, 30808, -4299, 1714, -707, 257, -74, 15, -1,
		0, 2, -18, 78, -248, 648, -1549, 4259, 31887, -3167, 1246, -516, 190, -56, 12, -1,
		0, 1, -8, 37, -118, 312, -747, 2006, 32546, -1731, 671, -279, 103, -31, 7, -1,
		0, 0, 0, 0, 0, 0, 0, 0, 32767, 0, 0, 0, 0, 0, 0, 0,
	}
)

type Sample struct {
	volume, panning, relNote, fineTune int
	c2Rate                             C2Rate
	loopStart, loopLength              int
	sampleData                         []int16
	name                               string
}

func (this *Sample) looped() bool {
	return this.loopLength > 1
}

func (this *Sample) normaliseSampleIdx(sampleIdx int) int {
	loopOffset := sampleIdx - this.loopStart
	if loopOffset > 0 {
		sampleIdx = this.loopStart
		if this.loopLength > 1 {
			sampleIdx += loopOffset % this.loopLength
		}
	}
	return sampleIdx
}

func (this *Sample) setSampleData(sampleData []int16, loopStart, loopLength int, pingPong bool) {
	sampleLength := len(sampleData)
	// Fix loop if necessary.
	if loopStart < 0 || loopStart > sampleLength {
		loopStart = sampleLength
	}
	if loopLength < 0 || (loopStart+loopLength) > sampleLength {
		loopLength = sampleLength - loopStart
	}
	sampleLength = loopStart + loopLength
	// Compensate for sinc-interpolator delay.
	loopStart += DELAY
	// Allocate new sample.
	newSampleLength := DELAY + sampleLength + FILTER_TAPS
	if pingPong {
		newSampleLength += loopLength
	}
	newSampleData := make([]int16, newSampleLength)
	copy(newSampleData[DELAY:], sampleData[:sampleLength])

	sampleData = newSampleData
	if pingPong {
		// Calculate reversed loop.
		loopEnd := loopStart + loopLength
		for idx := 0; idx < loopLength; idx++ {
			sampleData[loopEnd+idx] = sampleData[loopEnd-idx-1]
		}
		loopLength *= 2
	}
	// Extend loop for sinc interpolator.
	idx := loopStart + loopLength
	end := idx + FILTER_TAPS
	for ; idx < end; idx++ {
		sampleData[idx] = sampleData[idx-loopLength]
	}
	this.sampleData = sampleData
	this.loopStart = loopStart
	this.loopLength = loopLength
}

func (this *Sample) resampleLinear(sampleIdx, sampleFrac, step,
	leftGain, rightGain int, mixBuffer []int32, offset, length int) {
	loopLen := this.loopLength
	loopEnd := this.loopStart + loopLen
	sampleIdx += DELAY
	if sampleIdx >= loopEnd {
		sampleIdx = this.normaliseSampleIdx(sampleIdx)
	}
	data := this.sampleData
	outIdx := offset << 1
	outEnd := (offset + length) << 1

	for outIdx < outEnd {
		if sampleIdx >= loopEnd {
			if loopLen < 2 {
				break
			}
			for sampleIdx >= loopEnd {
				sampleIdx -= loopLen
			}
		}
		c := data[sampleIdx]
		m := int(data[sampleIdx+1]) - int(c)
		y := (int(m) * sampleFrac >> FP_SHIFT) + int(c)
		mixBuffer[outIdx] += int32(y * leftGain >> FP_SHIFT)
		outIdx++
		mixBuffer[outIdx] += int32(y * rightGain >> FP_SHIFT)
		outIdx++

		sampleFrac += step
		sampleIdx += sampleFrac >> FP_SHIFT
		sampleFrac &= FP_MASK
	}
}

func (this *Sample) resampleNearest(sampleIdx, sampleFrac, step,
	leftGain, rightGain int, mixBuffer []int32, offset, length int) {
	loopLen := this.loopLength
	loopEnd := this.loopStart + loopLen
	sampleIdx += DELAY
	if sampleIdx >= loopEnd {
		sampleIdx = this.normaliseSampleIdx(sampleIdx)
	}
	data := this.sampleData
	outIdx := offset << 1
	outEnd := (offset + length) << 1

	for outIdx < outEnd {
		if sampleIdx >= loopEnd {
			if loopLen < 2 {
				break
			}
			for sampleIdx >= loopEnd {
				sampleIdx -= loopLen
			}
		}
		y := int(data[sampleIdx])
		mixBuffer[outIdx] += int32(y * leftGain >> FP_SHIFT)
		outIdx++
		mixBuffer[outIdx] += int32(y * rightGain >> FP_SHIFT)
		outIdx++
		sampleFrac += step
		sampleIdx += sampleFrac >> FP_SHIFT
		sampleFrac &= FP_MASK
	}
}

func (this *Sample) resampleSinc(sampleIdx, sampleFrac, step,
	leftGain, rightGain int, mixBuffer []int32, offset, length int) {
	loopLen := this.loopLength
	loopEnd := this.loopStart + loopLen
	if sampleIdx >= loopEnd {
		sampleIdx = this.normaliseSampleIdx(sampleIdx)
	}
	data := this.sampleData
	outIdx := offset << 1
	outEnd := (offset + length) << 1

	for outIdx < outEnd {
		if sampleIdx >= loopEnd {
			if loopLen < 2 {
				break
			}
			for sampleIdx >= loopEnd {
				sampleIdx -= loopLen
			}
		}
		tableIdx := (sampleFrac >> TABLE_INTERP_SHIFT) << LOG2_FILTER_TAPS
		a1, a2 := int(0), int(0)
		a1 = int(SINC_TABLE[tableIdx+0]) * int(data[sampleIdx+0])
		a1 += int(SINC_TABLE[tableIdx+1]) * int(data[sampleIdx+1])
		a1 += int(SINC_TABLE[tableIdx+2]) * int(data[sampleIdx+2])
		a1 += int(SINC_TABLE[tableIdx+3]) * int(data[sampleIdx+3])
		a1 += int(SINC_TABLE[tableIdx+4]) * int(data[sampleIdx+4])
		a1 += int(SINC_TABLE[tableIdx+5]) * int(data[sampleIdx+5])
		a1 += int(SINC_TABLE[tableIdx+6]) * int(data[sampleIdx+6])
		a1 += int(SINC_TABLE[tableIdx+7]) * int(data[sampleIdx+7])
		a1 += int(SINC_TABLE[tableIdx+8]) * int(data[sampleIdx+8])
		a1 += int(SINC_TABLE[tableIdx+9]) * int(data[sampleIdx+9])
		a1 += int(SINC_TABLE[tableIdx+10]) * int(data[sampleIdx+10])
		a1 += int(SINC_TABLE[tableIdx+11]) * int(data[sampleIdx+11])
		a1 += int(SINC_TABLE[tableIdx+12]) * int(data[sampleIdx+12])
		a1 += int(SINC_TABLE[tableIdx+13]) * int(data[sampleIdx+13])
		a1 += int(SINC_TABLE[tableIdx+14]) * int(data[sampleIdx+14])
		a1 += int(SINC_TABLE[tableIdx+15]) * int(data[sampleIdx+15])
		a2 = int(SINC_TABLE[tableIdx+16]) * int(data[sampleIdx+0])
		a2 += int(SINC_TABLE[tableIdx+17]) * int(data[sampleIdx+1])
		a2 += int(SINC_TABLE[tableIdx+18]) * int(data[sampleIdx+2])
		a2 += int(SINC_TABLE[tableIdx+19]) * int(data[sampleIdx+3])
		a2 += int(SINC_TABLE[tableIdx+20]) * int(data[sampleIdx+4])
		a2 += int(SINC_TABLE[tableIdx+21]) * int(data[sampleIdx+5])
		a2 += int(SINC_TABLE[tableIdx+22]) * int(data[sampleIdx+6])
		a2 += int(SINC_TABLE[tableIdx+23]) * int(data[sampleIdx+7])
		a2 += int(SINC_TABLE[tableIdx+24]) * int(data[sampleIdx+8])
		a2 += int(SINC_TABLE[tableIdx+25]) * int(data[sampleIdx+9])
		a2 += int(SINC_TABLE[tableIdx+26]) * int(data[sampleIdx+10])
		a2 += int(SINC_TABLE[tableIdx+27]) * int(data[sampleIdx+11])
		a2 += int(SINC_TABLE[tableIdx+28]) * int(data[sampleIdx+12])
		a2 += int(SINC_TABLE[tableIdx+29]) * int(data[sampleIdx+13])
		a2 += int(SINC_TABLE[tableIdx+30]) * int(data[sampleIdx+14])
		a2 += int(SINC_TABLE[tableIdx+31]) * int(data[sampleIdx+15])
		a1 >>= FP_SHIFT
		a2 >>= FP_SHIFT
		y := a1 + ((a2 - a1) * ((sampleFrac) & TABLE_INTERP_MASK) >> TABLE_INTERP_SHIFT)
		mixBuffer[outIdx] += int32(y * leftGain >> FP_SHIFT)
		outIdx++
		mixBuffer[outIdx] += int32(y * rightGain >> FP_SHIFT)
		outIdx++
		sampleFrac += step
		sampleIdx += sampleFrac >> FP_SHIFT
		sampleFrac &= FP_MASK
	}
}
