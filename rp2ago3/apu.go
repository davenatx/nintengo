package rp2ago3

type Control uint8
type Status uint8

type PulseFlag uint32

const (
	Duty PulseFlag = 1 << iota
	PulseEnvelopeLoopLengthCounterHalt
	PulseConstantVolume
	PulseVolumeEnvelope
	SweepEnabled
	SweepPeriod
	SweepNegate
	SweepShift
	PulseTimerLow
	PulseLengthCounterLoad
	PulseTimerHigh
)

type TriangleFlag uint32

const (
	LengthCounterHaltLinearCounterControl = 1 << iota
	LinearCounterLoad
	TriangleTimerLow
	TriangleLengthCounterLoad
	TriangleTimerHigh
)

type NoiseFlag uint32

const (
	NoiseEnvelopeLoopLengthCounterHalt NoiseFlag = 1 << iota
	NoiseConstantVolume
	NoiseVolumeEnvelope
	LoopNoise
	NoisePeriod
	NoiseLengthCounterLoad
)

type DMCFlag uint32

const (
	IRQEnable DMCFlag = 1 << iota
	Loop
	Frequency
	LoadCounter
	SampleAddress
	SampleLength
)

type ControlFlag uint8

const (
	EnablePulseChannel1 ControlFlag = 1 << iota
	EnablePulseChannel2
	EnableTriangle
	EnableNoise
	EnableDMC
	_
	_
	_
)

type StatusFlag uint8

const (
	Pulse1LengthCounterNotZero StatusFlag = 1 << iota
	Pulse2LengthCounterNotZero
	TriangleLengthCounterNotZero
	NoiseLengthCounterNotZero
	DMCActive
	_
	FrameInterrupt
	DMCInterrupt
)

type FrameCounterFlag uint8

const (
	IRQInhibit FrameCounterFlag = 1 << iota
	Mode
)

type Registers struct {
	Control Control
	Status  Status
}

type APU struct {
	Registers Registers

	Pulse1       Pulse
	Pulse2       Pulse
	Triangle     Triangle
	Noise        Noise
	DMC          DMC
	FrameCounter FrameCounter

	Cycles       uint64
	TargetCycles uint64

	HipassStrong int64
	HipassWeak   int64
	pulseLUT     [31]float64
	tndLUT       [203]float64

	Interrupt func(state bool)
}

func NewAPU(interrupt func(bool)) *APU {
	apu := &APU{
		TargetCycles: 1789773 / 44100,
		Interrupt:    interrupt,
		Noise: Noise{
			PeriodLUT: [16]int16{
				// NTSC
				4, 8, 16, 32, 64, 96, 128, 160, 202,
				254, 380, 508, 762, 1016, 2034, 4068,
				// PAL
				// 4, 8, 14, 30, 60, 88, 118, 148, 188,
				// 236, 354, 472, 708, 944, 1890, 3778,
			},
			LengthCounterLUT: [32]uint8{
				0x0a, 0xfe, 0x14, 0x02,
				0x28, 0x04, 0x50, 0x06,
				0xa0, 0x08, 0x3c, 0x0a,
				0x0e, 0x0c, 0x1a, 0x0e,
				0x0c, 0x10, 0x18, 0x12,
				0x30, 0x14, 0x60, 0x16,
				0xc0, 0x18, 0x48, 0x1a,
				0x10, 0x1c, 0x20, 0x1e,
			},
		},
		Triangle: Triangle{
			LengthCounterLUT: [32]uint8{
				0x0a, 0xfe, 0x14, 0x02,
				0x28, 0x04, 0x50, 0x06,
				0xa0, 0x08, 0x3c, 0x0a,
				0x0e, 0x0c, 0x1a, 0x0e,
				0x0c, 0x10, 0x18, 0x12,
				0x30, 0x14, 0x60, 0x16,
				0xc0, 0x18, 0x48, 0x1a,
				0x10, 0x1c, 0x20, 0x1e,
			},
		},
	}

	for i := 0; i < len(apu.pulseLUT); i++ {
		apu.pulseLUT[i] = 95.52 / (8128.0/float64(i) + 100.0)
	}

	for i := 0; i < len(apu.tndLUT); i++ {
		apu.tndLUT[i] = 163.67 / (24329.0/float64(i) + 100.0)
	}

	return apu
}

func (apu *APU) Reset() {
	apu.Registers.Control = 0x00
	apu.Registers.Status = 0x00
	apu.Noise.Shift = 0x0001
}

func (apu *APU) Mappings(which Mapping) (fetch, store []uint16) {
	switch which {
	case CPU:
		fetch = []uint16{0x4015}
		store = []uint16{
			0x4000, 0x4001, 0x4002, 0x4003, 0x4004,
			0x4005, 0x4006, 0x4007, 0x4008, 0x400a,
			0x400b, 0x400c, 0x400e, 0x400f, 0x4010,
			0x4011, 0x4012, 0x4013, 0x4015, 0x4017,
		}
	}

	return
}

func (apu *APU) Fetch(address uint16) (value uint8) {
	switch address {
	// Status
	case 0x4015:
		value = uint8(apu.Registers.Status)
		apu.Registers.Status &= Status(^FrameInterrupt)
	}

	return
}

func (apu *APU) Store(address uint16, value uint8) (oldValue uint8) {
	switch {
	// Pulse 1 channel
	case address >= 0x4000 && address <= 0x4003:
		oldValue = apu.Pulse1.Store(address-0x4000, value)
	// Pulse 2 channel
	case address >= 0x4004 && address <= 0x4007:
		oldValue = apu.Pulse2.Store(address-0x4004, value)
	// Triangle channel
	case address >= 0x4008 && address <= 0x400b:
		index := address - 0x4008

		switch address {
		case 0x4009: // 0x4009 is not mapped
			break
		case 0x400b:
			fallthrough
		case 0x400a:
			index--
			fallthrough
		case 0x4008:
			oldValue = apu.Triangle.Store(index, value)
		}
	// Noise channel
	case address >= 0x400c && address <= 0x400f:
		index := address - 0x400c

		switch address {
		case 0x400d: // 0x400d is not mapped
			break
		case 0x400f:
			fallthrough
		case 0x400e:
			index--
			fallthrough
		case 0x400c:
			oldValue = apu.Noise.Store(index, value)
		}
	// DMC channel
	case address >= 0x4010 && address <= 0x4013:
		oldValue = apu.DMC.Store(address-0x4010, value)
	// Control
	case address == 0x4015:
		oldValue = uint8(apu.Registers.Control)
		apu.Registers.Control = Control(value)
		apu.Registers.Status &= Status(^DMCInterrupt)

		if !apu.control(EnableNoise) {
			apu.Noise.LengthCounter = 0
		}
	// Frame counter
	case address == 0x4017:
		var execute bool

		if oldValue, execute = apu.FrameCounter.Store(value); execute {
			apu.ExecuteFrameCounter()
		}

	}

	return
}

func (apu *APU) hipassStrong(s int16) int16 {
	HiPassStrong := int64(225574)

	apu.HipassStrong += (((int64(s) << 16) - (apu.HipassStrong >> 16)) * HiPassStrong) >> 16
	return int16(int64(s) - (apu.HipassStrong >> 32))
}

func (apu *APU) hipassWeak(s int16) int16 {
	HiPassWeak := int64(57593)

	apu.HipassWeak += (((int64(s) << 16) - (apu.HipassWeak >> 16)) * HiPassWeak) >> 16
	return int16(int64(s) - (apu.HipassWeak >> 32))
}

func (apu *APU) Sample() (sample int16) {
	pulse := apu.pulseLUT[apu.Pulse1.Sample()+apu.Pulse2.Sample()]
	tnd := apu.tndLUT[(3*apu.Triangle.Sample())+(2*apu.Noise.Sample())+apu.DMC.Sample()]

	sample = int16((pulse + tnd) * 40000)
	sample = apu.hipassStrong(sample)
	sample = apu.hipassWeak(sample)

	return
}

func (apu *APU) control(flag ControlFlag, state ...bool) (value bool) {
	if len(state) == 0 {
		if (apu.Registers.Control & Control(flag)) != 0 {
			value = true
		}
	} else {
		value = state[0]

		if !value {
			apu.Registers.Control &= Control(^flag)
		} else {
			apu.Registers.Control |= Control(flag)
		}
	}

	return
}

func (apu *APU) status(flag StatusFlag, state ...bool) (value bool) {
	if len(state) == 0 {
		if (apu.Registers.Status & Status(flag)) != 0 {
			value = true
		}
	} else {
		value = state[0]

		if !value {
			apu.Registers.Status &= Status(^flag)
		} else {
			apu.Registers.Status |= Status(flag)
		}
	}

	return
}

func (apu *APU) ExecuteFrameCounter() {
	if changed, step := apu.FrameCounter.Clock(); changed {
		switch step {
		case 1:
			// clock env & tri's linear counter
			apu.Noise.ClockEnvelope()
			apu.Triangle.LinearCounter.Clock()
		case 2:
			// clock env & tri's linear counter
			apu.Noise.ClockEnvelope()
			apu.Triangle.LinearCounter.Clock()

			// clock length counters & sweep units
			apu.Noise.ClockLengthCounter()
		case 3:
			// clock env & tri's linear counter
			apu.Noise.ClockEnvelope()
			apu.Triangle.LinearCounter.Clock()
		case 4:
			if apu.FrameCounter.NumSteps == 4 {
				// clock env & tri's linear counter
				apu.Noise.ClockEnvelope()
				apu.Triangle.LinearCounter.Clock()

				// clock length counters & sweep units
				apu.Noise.ClockLengthCounter()

				// set frame interrupt flag if interrupt inhibit is clear
				if !apu.FrameCounter.IRQInhibit {
					apu.Registers.Status |= Status(FrameInterrupt)
				}
			}
		case 5:
			if apu.FrameCounter.NumSteps == 5 {
				// clock env & tri's linear counter
				apu.Noise.ClockEnvelope()
				apu.Triangle.LinearCounter.Clock()

				// clock length counters & sweep units
				apu.Noise.ClockLengthCounter()
			}
		}

		if step == apu.FrameCounter.NumSteps {
			apu.FrameCounter.Reset()
		}
	}
}

func (apu *APU) Execute() (sample int16, haveSample bool) {
	apu.ExecuteFrameCounter()

	if apu.control(EnableNoise) {
		apu.Noise.ClockDivider()
	}

	if apu.Cycles++; apu.Cycles == apu.TargetCycles {
		sample = apu.Sample()
		haveSample = true

		apu.Cycles = 0

		apu.TargetCycles ^= 0x1
	}

	return
}

type Pulse struct {
	Registers [4]uint8
}

func (pulse *Pulse) Store(index uint16, value uint8) (oldValue uint8) {
	oldValue = pulse.Registers[index]
	pulse.Registers[index] = value

	return
}

func (pulse *Pulse) registers(flag PulseFlag, state ...uint8) (value uint8) {
	if len(state) == 0 {
		switch flag {
		case Duty:
			value = pulse.Registers[0] >> 6
		case PulseEnvelopeLoopLengthCounterHalt:
			value = (pulse.Registers[0] >> 5) & 0x01
		case PulseConstantVolume:
			value = (pulse.Registers[0] >> 4) & 0x01
		case PulseVolumeEnvelope:
			value = pulse.Registers[0] & 0x0f
		case SweepEnabled:
			value = pulse.Registers[1] >> 7
		case SweepPeriod:
			value = (pulse.Registers[1] >> 4) & 0x07
		case SweepNegate:
			value = (pulse.Registers[1] >> 3) & 0x01
		case SweepShift:
			value = (pulse.Registers[1] & 0x07)
		case PulseTimerLow:
			value = pulse.Registers[2]
		case PulseLengthCounterLoad:
			value = pulse.Registers[3] >> 3
		case PulseTimerHigh:
			value = pulse.Registers[3] & 0x07
		}
	} else {
		value = state[0]

		switch flag {
		case Duty:
			value = (pulse.Registers[0] & 0x3f) | ((value & 0x03) << 6)
		case PulseEnvelopeLoopLengthCounterHalt:
			value = (pulse.Registers[0] & 0xdf) | ((value & 0x01) << 5)
		case PulseConstantVolume:
			value = (pulse.Registers[0] & 0xef) | ((value & 0x01) << 4)
		case PulseVolumeEnvelope:
			value = (pulse.Registers[0] & 0xf0) | (value & 0x0f)
		case SweepEnabled:
			value = (pulse.Registers[1] & 0x7f) | ((value & 0x01) << 7)
		case SweepPeriod:
			value = (pulse.Registers[1] & 0x8f) | ((value & 0x07) << 4)
		case SweepNegate:
			value = (pulse.Registers[1] & 0xf7) | ((value & 0x01) << 3)
		case SweepShift:
			value = (pulse.Registers[1] & 0xf8) | (value & 0x07)
		case PulseTimerLow:
			pulse.Registers[2] = value
		case PulseLengthCounterLoad:
			value = (pulse.Registers[3] & 0x07) | ((value & 0x1f) << 3)
		case PulseTimerHigh:
			value = (pulse.Registers[3] & 0xf8) | (value & 0x07)
		}
	}

	return
}

func (pulse *Pulse) Sample() (sample int16) {
	return
}

type Triangle struct {
	Registers [3]uint8

	Divider          Divider
	LinearCounter    LinearCounter
	Sequencer        Sequencer
	LengthCounter    uint8
	LengthCounterLUT [32]uint8
}

func (triangle *Triangle) Store(index uint16, value uint8) (oldValue uint8) {
	oldValue = triangle.Registers[index]
	triangle.Registers[index] = value

	return
}

func (triangle *Triangle) registers(flag TriangleFlag, state ...uint8) (value uint8) {
	if len(state) == 0 {
		switch flag {
		case LengthCounterHaltLinearCounterControl:
			value = triangle.Registers[0] >> 7
		case LinearCounterLoad:
			value = triangle.Registers[0] & 0x7f
		case TriangleTimerLow:
			value = triangle.Registers[1]
		case TriangleLengthCounterLoad:
			value = triangle.Registers[2] >> 3
		case TriangleTimerHigh:
			value = triangle.Registers[2] & 0x07
		}
	} else {
		value = state[0]

		switch flag {
		case LengthCounterHaltLinearCounterControl:
			value = (triangle.Registers[0] & 0x7f) | ((value & 0x01) << 7)
		case LinearCounterLoad:
			value = (triangle.Registers[0] & 0x80) | (value & 0x7f)
		case TriangleTimerLow:
			triangle.Registers[1] = value
		case TriangleLengthCounterLoad:
			value = (triangle.Registers[2] & 0x07) | ((value & 0x1f) << 3)
		case TriangleTimerHigh:
			value = (triangle.Registers[2] & 0xf8) | (value & 0x07)
		}
	}

	return
}

func (triangle *Triangle) Sample() (sample int16) {
	return
}

type Noise struct {
	Registers [3]uint8

	Envelope         Envelope
	Divider          Divider
	Shift            uint16
	LengthCounter    uint8
	LengthCounterLUT [32]uint8
	PeriodLUT        [16]int16
}

func (noise *Noise) Store(index uint16, value uint8) (oldValue uint8) {
	oldValue = noise.Registers[index]
	noise.Registers[index] = value

	switch index {
	// $400c
	case 0:
		noise.Envelope.Counter = noise.registers(NoiseVolumeEnvelope)
		noise.Envelope.Loop = noise.registers(LoopNoise) != 0
	// $400e
	case 1:
		noise.Divider.Period = noise.PeriodLUT[noise.registers(NoisePeriod)]
		noise.Divider.Reload()
	// $400f
	case 2:
		noise.LengthCounter = noise.LengthCounterLUT[noise.registers(NoiseLengthCounterLoad)]
	}

	return
}

func (noise *Noise) registers(flag NoiseFlag, state ...uint8) (value uint8) {
	if len(state) == 0 {
		switch flag {
		case NoiseEnvelopeLoopLengthCounterHalt:
			value = (noise.Registers[0] >> 5) & 0x01
		case NoiseConstantVolume:
			value = (noise.Registers[0] >> 4) & 0x01
		case NoiseVolumeEnvelope:
			value = noise.Registers[0] & 0x0f
		case LoopNoise:
			value = noise.Registers[1] >> 7
		case NoisePeriod:
			value = noise.Registers[1] & 0x0f
		case NoiseLengthCounterLoad:
			value = noise.Registers[2] >> 3
		}
	} else {
		value = state[0]

		switch flag {
		case NoiseEnvelopeLoopLengthCounterHalt:
			value = (noise.Registers[0] & 0xdf) | ((value & 0x01) << 5)
		case NoiseConstantVolume:
			value = (noise.Registers[0] & 0xef) | ((value & 0x01) << 4)
		case NoiseVolumeEnvelope:
			value = (noise.Registers[0] & 0xf0) | (value & 0x0f)
		case LoopNoise:
			value = (noise.Registers[1] & 0x7f) | ((value & 0x01) << 7)
		case NoisePeriod:
			value = (noise.Registers[1] & 0xf0) | (value & 0x0f)
		case NoiseLengthCounterLoad:
			value = (noise.Registers[2] & 0x07) | ((value & 0x1f) << 3)
		}
	}

	return
}

func (noise *Noise) ClockLengthCounter() {
	if noise.registers(NoiseEnvelopeLoopLengthCounterHalt) != 0 &&
		noise.LengthCounter > 0 {
		noise.LengthCounter--
	}
}

func (noise *Noise) ClockEnvelope() {
	noise.Envelope.Clock()
}

func (noise *Noise) ClockDivider() {
	var tmp uint16

	if noise.Divider.Clock() {
		// fmt.Printf("APU: Noise: timer clocked\n")

		if noise.registers(LoopNoise) == 0 {
			tmp = 5
		} else {
			tmp = 1
		}

		bit := (noise.Shift >> tmp) & 0x0001
		feedback := (noise.Shift & 0x0001) ^ bit

		noise.Shift = (noise.Shift >> 1) | (feedback << 14)
	}
}

func (noise *Noise) Sample() (sample int16) {
	if (noise.Shift&0x0001) == 0 && noise.LengthCounter != 0 {
		if noise.registers(NoiseConstantVolume) == 0 {
			sample = int16(noise.Envelope.Counter)
		} else {
			sample = int16(noise.registers(NoiseVolumeEnvelope))
		}
	}

	return
}

type DMC struct {
	Registers [4]uint8
}

func (dmc *DMC) Store(index uint16, value uint8) (oldValue uint8) {
	oldValue = dmc.Registers[index]
	dmc.Registers[index] = value

	return
}

func (dmc *DMC) registers(flag DMCFlag, state ...uint8) (value uint8) {
	if len(state) == 0 {
		switch flag {
		case IRQEnable:
			value = dmc.Registers[0] >> 7
		case Loop:
			value = (dmc.Registers[0] >> 6) & 0x01
		case Frequency:
			value = dmc.Registers[0] & 0x0f
		case LoadCounter:
			value = dmc.Registers[1] & 0x7f
		case SampleAddress:
			value = dmc.Registers[2]
		case SampleLength:
			value = dmc.Registers[3]
		}
	} else {
		value = state[0]

		switch flag {
		case IRQEnable:
			value = (dmc.Registers[0] & 0x7f) | ((value & 0x01) << 7)
		case Loop:
			value = (dmc.Registers[0] & 0xbf) | ((value & 0x01) << 6)
		case Frequency:
			value = (dmc.Registers[0] & 0xf0) | (value & 0x0f)
		case LoadCounter:
			value = (dmc.Registers[1] & 0x80) | (value & 0x7f)
		case SampleAddress:
			dmc.Registers[2] = value
		case SampleLength:
			dmc.Registers[3] = value
		}
	}

	return
}

func (dmc *DMC) Sample() (sample int16) {
	return
}

type FrameCounter struct {
	Register   uint8
	IRQInhibit bool
	NumSteps   uint8
	Step       uint8
	Cycles     uint16
}

func (frameCounter *FrameCounter) Reset() {
	frameCounter.Cycles = 0
	frameCounter.Step = 0
}

func (frameCounter *FrameCounter) Store(value uint8) (oldValue uint8, execute bool) {
	oldValue = frameCounter.Register
	frameCounter.Register = value

	oldValue = uint8(frameCounter.Register)
	frameCounter.Register = value

	frameCounter.Reset()

	if frameCounter.register(Mode) == 5 {
		execute = true
	}

	return
}

func (frameCounter *FrameCounter) register(flag FrameCounterFlag, state ...uint8) (value uint8) {
	if len(state) == 0 {
		switch flag {
		case Mode:
			switch uint8(frameCounter.Register >> 7) {
			case 0:
				value = 4
			case 1:
				value = 5
			}
		case IRQInhibit:
			value = uint8(frameCounter.Register>>6) & 0x01
		}
	} else {
		value = state[0]

		switch flag {
		case Mode:
			switch value {
			case 4:
				frameCounter.Register &= 0x7f
			case 5:
				frameCounter.Register |= 0x80
			}
		case IRQInhibit:
			switch value {
			case 0:
				frameCounter.Register &= 0xbf
			case 1:
				frameCounter.Register |= 0x40
			}
		}
	}

	return
}

func (frameCounter *FrameCounter) Clock() (changed bool, newStep uint8) {
	// 2 CPU cycles = 1 APU cycle
	frameCounter.Cycles++

	oldStep := frameCounter.Step

	switch frameCounter.Cycles {
	case 3729 * 2, 7457 * 2, 11186 * 2,
		14915 * 2, 18641 * 2:
		frameCounter.Step++
	}

	newStep = frameCounter.Step

	if oldStep != newStep {
		changed = true
	}

	return
}

type Sequencer struct {
	Values []uint8
	Index  int
	Output uint8
}

func (sequencer *Sequencer) Clock() (output uint8) {
	sequencer.Output = sequencer.Values[sequencer.Index]
	output = sequencer.Output

	sequencer.Index++

	if sequencer.Index == len(sequencer.Values) {
		sequencer.Index = 0
	}

	return
}

func (sequencer *Sequencer) Reset() {
	sequencer.Index = 0
}

type LinearCounter struct {
	Control     bool
	Halt        bool
	ReloadValue uint8
	Counter     uint8
}

func (linearCounter *LinearCounter) Clock() (counter uint8) {
	if linearCounter.Halt {
		linearCounter.Counter = linearCounter.ReloadValue
	} else if linearCounter.Counter > 0 {
		linearCounter.Counter--
	}

	if !linearCounter.Control {
		linearCounter.Halt = false
	}

	counter = linearCounter.Counter

	return
}

func (linearCounter *LinearCounter) Reset() {
	linearCounter.Halt = false
	linearCounter.Counter = 0
	linearCounter.ReloadValue = 0
}

type Envelope struct {
	Start bool
	Loop  bool
	Divider
	Counter uint8
}

func (envelope *Envelope) Clock() (output uint8) {
	if envelope.Start {
		envelope.Start = false
		envelope.Counter = 0x0f
		envelope.Divider.Reload()
	} else if envelope.Divider.Clock() {
		if envelope.Counter > 0 {
			envelope.Counter--
		} else if envelope.Loop {
			envelope.Counter = 0x0f
		}
	}

	return
}

type Divider struct {
	Counter int16
	Period  int16
}

func (divider *Divider) Clock() (output bool) {
	divider.Counter--

	if divider.Counter == 0 {
		divider.Reload()
		output = true
	}

	return
}

func (divider *Divider) Reload() {
	divider.Counter = divider.Period
}
