package main

import (
	//"encoding/binary"
	"fmt"
	"github.com/vova616/go-openal/openal"
	"github.com/vova616/ibxmgo"
	"os"
	"time"
)

func main() {
	test := "./test2.mod"
	if len(os.Args) > 1 {
		test = os.Args[1]
	}

	f, e := os.Open(test)
	if e != nil {
		panic(e)
	}
	m, e := ibxmgo.Decode(f)
	if e != nil {
		panic(e)
	}

	ibxm, e := ibxmgo.NewIBXM(m, 48000)
	if e != nil {
		panic(e)
	}

	//f2, _ := os.Create("./output.raw")
	//ibxm.Dump(f2)
	//return
	duration := ibxm.SongDuration()

	data := make([]int32, ibxm.MixBufferLength())
	data2 := make([]int16, duration*2)

	s := time.Now()
	index := 0
	for {
		in, ended := ibxm.GetAudio(data)
		in *= 2
		for j := 0; j < in; j++ {
			x := data[j]
			if x > 32767 {
				x = 32767
			} else if x < -32768 {
				x = -32768
			}
			data2[index] = int16(x)
			index++
		}
		if ended {
			break
		}
	}

	fmt.Println(time.Since(s))

	//return
	device := openal.OpenDevice("")
	context := device.CreateContext()
	context.Activate()

	//listener := new(openal.Listener)

	source := openal.NewSource()
	source.SetPitch(1)
	source.SetGain(1)
	source.SetPosition(0, 0, 0)
	source.SetVelocity(0, 0, 0)
	source.SetLooping(false)

	buffer := openal.NewBuffer()

	buffer.SetDataInt(openal.FormatStereo16, data2, 48000)

	source.SetBuffer(buffer)
	source.Play()
	for source.State() == openal.Playing {

		//loop long enough to let the wave file finish

	}
	time.Sleep(time.Second)
	source.Pause()
	source.Stop()
	return
	context.Destroy()
	time.Sleep(time.Second)
}
