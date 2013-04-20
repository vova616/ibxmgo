package ibxmgo

type Pattern struct {
	numRows int
	data    []byte
}

func (this *Pattern) getNote(index int, note *Note) {
	offset := index * 5
	note.key = int(this.data[offset])
	note.instrument = int(this.data[offset+1])
	note.volume = int(this.data[offset+2])
	note.effect = int(this.data[offset+3])
	note.param = int(this.data[offset+4])
}

func NewPattern(numChannels, numRows int) *Pattern {
	return &Pattern{numRows, make([]byte, numChannels*numRows*5)}
}
