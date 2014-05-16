package nes

import (
	"errors"
	"fmt"
	"log"
	"runtime"

	"os"
	"runtime/pprof"

	"github.com/nwidger/nintengo/m65go2"
	"github.com/nwidger/nintengo/rp2ago3"
	"github.com/nwidger/nintengo/rp2cgo2"
)

type NES struct {
	running     bool
	cpu         *rp2ago3.RP2A03
	cpuDivisor  float32
	ppu         *rp2cgo2.RP2C02
	controllers *Controllers
	rom         ROM
	video       Video
	fps         *FPS
	recorder    Recorder
	options     *Options
}

type Options struct {
	Recorder   string
	CPUDecode  bool
	CPUProfile string
	MemProfile string
}

func NewNES(filename string, options *Options) (nes *NES, err error) {
	var video Video
	var recorder Recorder
	var cpuDivisor float32

	rom, err := NewROM(filename)

	if err != nil {
		err = errors.New(fmt.Sprintf("Error loading ROM: %v", err))
		return
	}

	switch rom.Region() {
	case NTSC:
		cpuDivisor = rp2ago3.NTSC_CPU_CLOCK_DIVISOR
	case PAL:
		cpuDivisor = rp2ago3.PAL_CPU_CLOCK_DIVISOR
	}

	cpu := rp2ago3.NewRP2A03()

	if options.CPUDecode {
		cpu.EnableDecode()
	}

	ctrls := NewControllers()

	video, err = NewSDLVideo()

	if err != nil {
		err = errors.New(fmt.Sprintf("Error creating video: %v", err))
		return
	}

	switch options.Recorder {
	case "none":
		// none
	case "jpeg":
		recorder, err = NewJPEGRecorder()
	case "gif":
		recorder, err = NewGIFRecorder()
	}

	if err != nil {
		err = errors.New(fmt.Sprintf("Error creating recorder: %v", err))
		return
	}

	ppu := rp2cgo2.NewRP2C02(cpu.InterruptLine(m65go2.Nmi))

	cpu.Memory.AddMappings(ppu, rp2ago3.CPU)
	cpu.Memory.AddMappings(rom, rp2ago3.CPU)
	cpu.Memory.AddMappings(ctrls, rp2ago3.CPU)

	ppu.Memory.AddMirrors(rom.Mirrors())
	ppu.Memory.AddMappings(rom, rp2ago3.PPU)

	nes = &NES{
		cpu:         cpu,
		cpuDivisor:  cpuDivisor,
		ppu:         ppu,
		rom:         rom,
		video:       video,
		fps:         NewFPS(DEFAULT_FPS),
		recorder:    recorder,
		controllers: ctrls,
		options:     options,
	}

	return
}

func (nes *NES) Reset() {
	nes.cpu.Reset()
	nes.ppu.Reset()
	nes.controllers.Reset()
}

type PressPause uint8
type PressReset uint8
type PressQuit uint8
type PressShowBackground uint8
type PressShowSprites uint8
type PressFPS100 uint8
type PressFPS75 uint8
type PressFPS50 uint8
type PressFPS25 uint8

func (nes *NES) pause() {
	for done := false; !done; {
		switch (<-nes.video.ButtonPresses()).(type) {
		case PressPause:
			done = true
		}
	}
}

func (nes *NES) route() {
	for nes.running {
		select {
		case e := <-nes.video.ButtonPresses():
			switch i := e.(type) {
			case PressButton:
				go func() {
					nes.controllers.Input() <- i
				}()
			case PressPause:
				nes.pause()
			case PressReset:
				nes.Reset()
			case PressQuit:
				nes.running = false
			case PressShowBackground:
				nes.ppu.ShowBackground = !nes.ppu.ShowBackground
			case PressShowSprites:
				nes.ppu.ShowSprites = !nes.ppu.ShowSprites
			case PressFPS100:
				nes.fps.SetRate(DEFAULT_FPS * 1.00)
			case PressFPS75:
				nes.fps.SetRate(DEFAULT_FPS * 0.75)
			case PressFPS50:
				nes.fps.SetRate(DEFAULT_FPS * 0.50)
			case PressFPS25:
				nes.fps.SetRate(DEFAULT_FPS * 0.25)
			}
		case e := <-nes.cpu.Cycles:
			go func() {
				nes.ppu.Cycles <- (e * nes.cpuDivisor)
				ok := <-nes.ppu.Cycles
				nes.cpu.Cycles <- ok
			}()
		case e := <-nes.ppu.Output:
			if nes.recorder != nil {
				nes.recorder.Input() <- e
			}

			go func() {
				nes.video.Input() <- e
				ok := <-nes.video.Input()
				nes.fps.Delay()
				nes.ppu.Output <- ok
			}()
		}
	}
}

func (nes *NES) Run() (err error) {
	fmt.Println(nes.rom)

	nes.Reset()

	nes.running = true

	go nes.controllers.Run()
	go nes.cpu.Run()
	go nes.ppu.Run()
	go nes.route()

	if nes.recorder != nil {
		go nes.recorder.Run()
	}

	runtime.LockOSThread()

	if nes.options.CPUProfile != "" {
		f, err := os.Create(nes.options.CPUProfile)

		if err != nil {
			log.Fatal(err)
		}

		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	nes.video.Run()

	if nes.recorder != nil {
		nes.recorder.Stop()
	}

	if nes.options.MemProfile != "" {
		f, err := os.Create(nes.options.MemProfile)

		if err != nil {
			log.Fatal(err)
		}

		pprof.WriteHeapProfile(f)
		f.Close()
	}

	return
}