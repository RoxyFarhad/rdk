//go:build linux

// These tests will only run on Linux! Viam's automated build system on Github uses Linux, though,
// so they should run on every PR. We made the tests Linux-only because this entire package is
// Linux-only, and building non-Linux support solely for the test meant that the code tested might
// not be the production code.
package genericlinux

import (
	"context"
	"testing"
	"time"

	"github.com/edaniels/golog"
	commonpb "go.viam.com/api/common/v1"
	"go.viam.com/test"
	"periph.io/x/conn/v3/gpio/gpiotest"

	"go.viam.com/rdk/components/board"
)

func TestRegisterBoard(t *testing.T) {
	RegisterBoard("test", map[int]GPIOBoardMapping{}, true)
}

func TestGenericLinux(t *testing.T) {
	ctx := context.Background()

	gp1 := &periphGpioPin{b: &sysfsBoard{
		logger: golog.NewTestLogger(t),
	}}

	t.Run("test empty sysfs board", func(t *testing.T) {
		test.That(t, gp1.b.GPIOPinNames(), test.ShouldBeNil)
		test.That(t, gp1.b.SPINames(), test.ShouldBeNil)
		_, err := gp1.PWM(ctx, nil)
		test.That(t, err, test.ShouldNotBeNil)
		_, err = gp1.b.GPIOPinByName("10")
		test.That(t, err, test.ShouldNotBeNil)
	})

	boardSPIs := map[string]*spiBus{
		"closed": {
			openHandle: &spiHandle{bus: &spiBus{}, isClosed: true},
		},
		"open": {
			openHandle: &spiHandle{bus: &spiBus{}, isClosed: false},
		},
	}
	oneStr := "1"
	twoStr := "1"
	boardSPIs["closed"].bus.Store(&oneStr)
	boardSPIs["closed"].openHandle.bus.bus.Store(&oneStr)
	boardSPIs["open"].bus.Store(&twoStr)
	boardSPIs["open"].openHandle.bus.bus.Store(&twoStr)

	gp2 := &periphGpioPin{
		b: &sysfsBoard{
			Named:        board.Named("foo").AsNamed(),
			gpioMappings: nil,
			spis:         boardSPIs,
			analogs:      map[string]*wrappedAnalog{"an": {}},
			pwms: map[string]pwmSetting{
				"10": {dutyCycle: 1, frequency: 1},
			},
			logger:    golog.NewTestLogger(t),
			cancelCtx: ctx,
			cancelFunc: func() {
			},
		},
		pinName:        "10",
		pin:            &gpiotest.Pin{N: "10", Num: 10},
		hwPWMSupported: false,
	}

	t.Run("test analogs spis i2cs digital-interrupts and gpio names", func(t *testing.T) {
		ans := gp2.b.AnalogReaderNames()
		test.That(t, ans, test.ShouldResemble, []string{"an"})

		an1, ok := gp2.b.AnalogReaderByName("an")
		test.That(t, an1, test.ShouldHaveSameTypeAs, &wrappedAnalog{})
		test.That(t, ok, test.ShouldBeTrue)

		an2, ok := gp2.b.AnalogReaderByName("missing")
		test.That(t, an2, test.ShouldBeNil)
		test.That(t, ok, test.ShouldBeFalse)

		sns := gp2.b.SPINames()
		test.That(t, len(sns), test.ShouldEqual, 2)

		sn1, ok := gp2.b.SPIByName("closed")
		test.That(t, sn1, test.ShouldHaveSameTypeAs, &spiBus{})
		test.That(t, ok, test.ShouldBeTrue)

		sn2, ok := gp2.b.SPIByName("missing")
		test.That(t, sn2, test.ShouldBeNil)
		test.That(t, ok, test.ShouldBeFalse)

		ins := gp2.b.I2CNames()
		test.That(t, ins, test.ShouldBeNil)

		in1, ok := gp2.b.I2CByName("in")
		test.That(t, in1, test.ShouldBeNil)
		test.That(t, ok, test.ShouldBeFalse)

		dns := gp2.b.DigitalInterruptNames()
		test.That(t, dns, test.ShouldBeNil)

		dn1, ok := gp2.b.DigitalInterruptByName("dn")
		test.That(t, dn1, test.ShouldBeNil)
		test.That(t, ok, test.ShouldBeFalse)

		gns := gp2.b.GPIOPinNames()
		test.That(t, gns, test.ShouldResemble, []string(nil))

		gn1, err := gp2.b.GPIOPinByName("10")
		test.That(t, err, test.ShouldNotBeNil)
		test.That(t, gn1, test.ShouldBeNil)
	})

	t.Run("test genericlinux gpio pin functionality", func(t *testing.T) {
		err := gp2.SetPWM(ctx, 50, nil)
		test.That(t, err, test.ShouldBeNil)

		err = gp2.SetPWMFreq(ctx, 1000, nil)
		test.That(t, err, test.ShouldBeNil)

		freq, err := gp2.PWMFreq(ctx, nil)
		test.That(t, err, test.ShouldBeNil)
		test.That(t, freq, test.ShouldEqual, 1000)

		duty, err := gp2.PWM(ctx, nil)
		test.That(t, err, test.ShouldBeNil)
		test.That(t, duty, test.ShouldEqual, 50)

		err = gp2.Set(ctx, true, nil)
		test.That(t, err, test.ShouldBeNil)

		high, err := gp2.Get(ctx, nil)
		test.That(t, err, test.ShouldBeNil)
		test.That(t, high, test.ShouldBeTrue)

		bs, err := gp2.b.Status(ctx, nil)
		test.That(t, err, test.ShouldBeNil)
		test.That(t, bs, test.ShouldResemble, &commonpb.BoardStatus{})

		bma := gp2.b.ModelAttributes()
		test.That(t, bma, test.ShouldResemble, board.ModelAttributes{})
	})

	t.Run("test spi functionality", func(t *testing.T) {
		spi1 := gp2.b.spis["closed"]
		spi2 := gp2.b.spis["open"]
		sph1, err := spi1.OpenHandle()
		test.That(t, sph1, test.ShouldHaveSameTypeAs, &spiHandle{})
		test.That(t, err, test.ShouldBeNil)
		sph2, err := spi2.OpenHandle()
		test.That(t, sph2, test.ShouldHaveSameTypeAs, &spiHandle{})
		test.That(t, err, test.ShouldBeNil)

		err = sph2.Close()
		test.That(t, err, test.ShouldBeNil)
		rx, err := sph2.Xfer(ctx, 1, "1", 1, []byte{})
		test.That(t, err.Error(), test.ShouldContainSubstring, "closed")
		test.That(t, rx, test.ShouldBeNil)
	})

	t.Run("test software pwm loop", func(t *testing.T) {
		newCtx, cancel := context.WithTimeout(ctx, time.Duration(10))
		defer cancel()
		gp2.b.softwarePWMLoop(newCtx, *gp2)

		gp2.b.pwms = map[string]pwmSetting{
			"10": {dutyCycle: 1, frequency: 1},
		}
		gp2.b.startSoftwarePWMLoop(*gp2)

		gp2.b.softwarePWMLoop(newCtx, *gp2)
	})

	t.Run("test getGPIOLine", func(t *testing.T) {
		_, err := gp2.b.getGPIOLine("10")
		test.That(t, err.Error(), test.ShouldContainSubstring, "no global pin")
	})
}

func TestConfigValidate(t *testing.T) {
	validConfig := Config{}

	validConfig.Analogs = []board.AnalogConfig{{}}
	_, err := validConfig.Validate("path")
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, err.Error(), test.ShouldContainSubstring, `path.analogs.0`)
	test.That(t, err.Error(), test.ShouldContainSubstring, `"name" is required`)

	validConfig.Analogs = []board.AnalogConfig{{Name: "bar"}}
	_, err = validConfig.Validate("path")
	test.That(t, err, test.ShouldBeNil)

	validConfig.DigitalInterrupts = []board.DigitalInterruptConfig{{}}
	_, err = validConfig.Validate("path")
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, err.Error(), test.ShouldContainSubstring, `path.digital_interrupts.0`)
	test.That(t, err.Error(), test.ShouldContainSubstring, `"name" is required`)

	validConfig.DigitalInterrupts = []board.DigitalInterruptConfig{{Name: "bar"}}
	_, err = validConfig.Validate("path")
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, err.Error(), test.ShouldContainSubstring, `path.digital_interrupts.0`)
	test.That(t, err.Error(), test.ShouldContainSubstring, `"pin" is required`)

	validConfig.DigitalInterrupts = []board.DigitalInterruptConfig{{Name: "bar", Pin: "3"}}
	_, err = validConfig.Validate("path")
	test.That(t, err, test.ShouldBeNil)
}
