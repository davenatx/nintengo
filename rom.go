package nintengo

import (
	"errors"
	"fmt"
	"github.com/nwidger/rp2ago3"
	"io/ioutil"
)

type Mirroring uint8

const (
	Horizontal Mirroring = iota
	Vertical
	FourScreen
)

type Region uint8

const (
	NTSC Region = iota
	PAL
)

type ROMFile struct {
	prgBanks    uint8
	chrBanks    uint8
	mirroring   Mirroring
	battery     bool
	trainer     bool
	fourScreen  bool
	vsCart      bool
	mapper      uint8
	ramBanks    uint8
	region      Region
	trainerData []uint8
	romBanks    [][]uint8
	vromBanks   [][]uint8
}

type ROM interface {
	rp2ago3.MappableMemory
	Region() Region
}

func NewROM(filename string) (rom ROM, err error) {
	buf, err := ioutil.ReadFile(filename)

	if err != nil {
		return
	}

	romf, err := NewROMFile(buf)

	if err != nil {
		return
	}

	switch romf.mapper {
	case 0:
		rom = NewNROM(romf)
	default:
		err = errors.New(fmt.Sprintf("Unsupported mapper type %v", romf.mapper))
	}

	return
}

func NewROMFile(buf []byte) (romf *ROMFile, err error) {
	var offset int

	if len(buf) < 16 {
		err = errors.New("Invalid ROM: Missing 16-byte header")
		return
	}

	if string(buf[0:3]) != "NES" || buf[3] != 0x1a {
		err = errors.New("Invalid ROM: Missing 'NES' constant in header")
		return
	}

	romf = &ROMFile{}

	i := 4

	for ; i < 10; i++ {
		byte := buf[i]

		switch i {
		case 4:
			romf.prgBanks = byte
		case 5:
			romf.chrBanks = byte
		case 6:
			for j := 0; j < 4; j++ {
				if byte&(0x01<<uint8(j)) != 0 {
					switch j {
					case 0:
						romf.mirroring = Vertical
					case 1:
						romf.battery = true
					case 2:
						romf.trainer = true
					case 3:
						romf.fourScreen = true
						romf.mirroring = FourScreen
					}
				}
			}

			romf.mapper = (byte >> 4) & 0x0f
		case 7:
			if byte&0x01 != 0 {
				romf.vsCart = true
			}

			romf.mapper |= byte & 0xf0

		case 8:
			romf.ramBanks = byte

			if romf.ramBanks == 0 {
				romf.ramBanks = 1
			}
		case 9:
			if byte&0x01 != 0 {
				romf.region = PAL
			}
		}
	}

	i += 6

	if romf.trainer {
		offset = 512

		if len(buf) < (i + offset) {
			romf = nil
			err = errors.New("Invalid ROM: EOF in trainer data")
			return
		}

		romf.trainerData = buf[i : i+offset]
		i += offset
	}

	offset = 1024 * 16

	if len(buf) < (i + (offset * int(romf.prgBanks))) {
		romf = nil
		err = errors.New("Invalid ROM: EOF in ROM bank data")
		return
	}

	romf.romBanks = make([][]uint8, romf.prgBanks)

	for n := 0; n < int(romf.prgBanks); n++ {
		romf.romBanks[n] = buf[i : i+offset]
		i += offset
	}

	offset = 1024 * 8

	if len(buf) < (i + (offset * int(romf.chrBanks))) {
		romf = nil
		err = errors.New("Invalid ROM: EOF in VROM bank data")
		return
	}

	romf.vromBanks = make([][]uint8, romf.chrBanks)

	for n := 0; n < int(romf.chrBanks); n++ {
		romf.vromBanks[n] = buf[i : i+offset]
		i += offset
	}

	return
}

func (romf *ROMFile) Region() Region {
	return romf.region
}