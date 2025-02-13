//go:build linux

package genericlinux

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/edaniels/golog"
	"github.com/mkch/gpio"

	rdkutils "go.viam.com/rdk/utils"
)

// adapted from https://github.com/NVIDIA/jetson-gpio (MIT License)

func noBoardError(modelName string) error {
	return fmt.Errorf("could not determine %q model", modelName)
}

// pwmChipData is a struct used solely within GetGPIOBoardMappings and its sub-pieces. It
// describes a PWM chip within sysfs.
type pwmChipData struct {
	Dir  string // Absolute path to pseudofile within sysfs to interact with this chip
	Npwm int    // Taken from the /npwm pseudofile in sysfs: number of lines on the chip
}

// GetGPIOBoardMappings attempts to find a compatible GPIOBoardMapping for the given board.
func GetGPIOBoardMappings(modelName string, boardInfoMappings map[string]BoardInformation) (map[int]GPIOBoardMapping, error) {
	pinDefs, err := getCompatiblePinDefs(modelName, boardInfoMappings)
	if err != nil {
		return nil, err
	}

	gpioChipsInfo, err := getGpioChipDefs(pinDefs)
	if err != nil {
		return nil, err
	}
	pwmChipsInfo, err := getPwmChipDefs(pinDefs)
	if err != nil {
		// Try continuing on without hardware PWM support. Many boards do not have it enabled by
		// default, and perhaps this robot doesn't even use it.
		golog.Global().Debugw("unable to find PWM chips, continuing without them", "error", err)
		pwmChipsInfo = map[string]pwmChipData{}
	}

	mapping, err := getBoardMapping(pinDefs, gpioChipsInfo, pwmChipsInfo)
	return mapping, err
}

// getCompatiblePinDefs returns a list of pin definitions, from the first BoardInformation struct
// that appears compatible with the machine we're running on.
func getCompatiblePinDefs(modelName string, boardInfoMappings map[string]BoardInformation) ([]PinDefinition, error) {
	compatibles, err := rdkutils.GetDeviceInfo(modelName)
	if err != nil {
		return nil, fmt.Errorf("error while getting hardware info %w", err)
	}

	var pinDefs []PinDefinition
	for _, info := range boardInfoMappings {
		for _, v := range info.Compats {
			if _, ok := compatibles[v]; ok {
				pinDefs = info.PinDefinitions
				break
			}
		}
	}

	if pinDefs == nil {
		return nil, noBoardError(modelName)
	}
	return pinDefs, nil
}

// A helper function: we read the contents of filePath and return its integer value.
func readIntFile(filePath string) (int, error) {
	//nolint:gosec
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return -1, err
	}
	resultInt64, err := strconv.ParseInt(strings.TrimSpace(string(contents)), 10, 64)
	return int(resultInt64), err
}

// getGpioChipDefs returns map of chip ngpio# to the corresponding gpio chip name.
func getGpioChipDefs(pinDefs []PinDefinition) (map[int]string, error) {
	allDevices := gpio.ChipDevices()
	ngpioToChipName := make(map[int]string, len(allDevices)) // maps chipNgpio -> string gpiochip#
	for _, dev := range allDevices {
		chip, err := gpio.OpenChip(dev)
		if err != nil {
			return nil, err
		}

		chipInfo, err := chip.Info()
		if err != nil {
			return nil, err
		}

		// should not have two chips with same ngpio #
		if _, ok := ngpioToChipName[int(chipInfo.NumLines)]; ok {
			golog.Global().Errorf("Board has multiple GPIO chips with the same ngpio value, %d!", chipInfo.NumLines)
		}
		ngpioToChipName[int(chipInfo.NumLines)] = chipInfo.Name
	}

	expectedNgpios := make(map[int]struct{}, len(pinDefs))
	for _, pinDef := range pinDefs {
		for n := range pinDef.GPIOChipRelativeIDs {
			expectedNgpios[n] = struct{}{} // get a "set" of all ngpio numbers on the board
		}
	}

	gpioChipsInfo := map[int]string{}
	// for each chip in the board config, find the right gpioChip dir
	for chipNgpio := range expectedNgpios {
		dir, ok := ngpioToChipName[chipNgpio]

		if !ok {
			return nil, fmt.Errorf("unknown GPIO device with ngpio %d",
				chipNgpio)
		}

		gpioChipsInfo[chipNgpio] = dir
	}

	return gpioChipsInfo, nil
}

func getPwmChipDefs(pinDefs []PinDefinition) (map[string]pwmChipData, error) {
	// First, collect the names of all relevant PWM chips with duplicates removed. Go doesn't have
	// native set objects, so we use a map whose values are ignored.
	pwmChipNames := make(map[string]struct{}, len(pinDefs))
	for _, pinDef := range pinDefs {
		if pinDef.PWMChipSysFSDir == "" {
			continue
		}
		pwmChipNames[pinDef.PWMChipSysFSDir] = struct{}{}
	}

	// Now, look for all chips whose names we found.
	pwmChipsInfo := map[string]pwmChipData{}
	const sysfsDir = "/sys/class/pwm"
	files, err := os.ReadDir(sysfsDir)
	if err != nil {
		return nil, err
	}

	for chipName := range pwmChipNames {
		found := false
		for _, file := range files {
			if !strings.HasPrefix(file.Name(), "pwmchip") {
				continue
			}

			// look at symlinks to find the correct chip
			symlink, err := os.Readlink(filepath.Join(sysfsDir, file.Name()))
			if err != nil {
				golog.Global().Errorw(
					"file is not symlink", "file", file.Name(), "err:", err)
				continue
			}

			if strings.Contains(symlink, chipName) {
				found = true
				chipPath := filepath.Join(sysfsDir, file.Name())
				npwm, err := readIntFile(filepath.Join(chipPath, "npwm"))
				if err != nil {
					return nil, err
				}

				pwmChipsInfo[chipName] = pwmChipData{Dir: chipPath, Npwm: npwm}
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("unable to find PWM device %s", chipName)
		}
	}
	return pwmChipsInfo, nil
}

func getBoardMapping(pinDefs []PinDefinition, gpioChipsInfo map[int]string,
	pwmChipsInfo map[string]pwmChipData,
) (map[int]GPIOBoardMapping, error) {
	data := make(map[int]GPIOBoardMapping, len(pinDefs))

	// For "use" on pins that don't have hardware PWMs
	dummyPwmInfo := pwmChipData{Dir: "", Npwm: -1}

	for _, pinDef := range pinDefs {
		key := pinDef.PinNumberBoard

		var ngpio int
		for n := range pinDef.GPIOChipRelativeIDs {
			ngpio = n
			break // each gpio pin should only be associated with one gpiochip in the config
		}

		gpioChipDir, ok := gpioChipsInfo[ngpio]
		if !ok {
			return nil, fmt.Errorf("unknown GPIO device for chip with ngpio %d, pin %d",
				ngpio, key)
		}

		pwmChipInfo, ok := pwmChipsInfo[pinDef.PWMChipSysFSDir]
		if ok {
			if pinDef.PWMID >= pwmChipInfo.Npwm {
				return nil, fmt.Errorf("too high PWM ID %d for pin %d (npwm is %d for chip %s)",
					pinDef.PWMID, key, pwmChipInfo.Npwm, pinDef.PWMChipSysFSDir)
			}
		} else {
			if pinDef.PWMChipSysFSDir == "" {
				// This pin isn't supposed to have hardware PWM support; all is well.
				pwmChipInfo = dummyPwmInfo
			} else {
				golog.Global().Errorw(
					"cannot find expected hardware PWM chip, continuing without it", "pin", key)
				pwmChipInfo = dummyPwmInfo
			}
		}

		chipRelativeID, ok := pinDef.GPIOChipRelativeIDs[ngpio]
		if !ok {
			chipRelativeID = pinDef.GPIOChipRelativeIDs[-1]
		}

		data[key] = GPIOBoardMapping{
			GPIOChipDev:    gpioChipDir,
			GPIO:           chipRelativeID,
			GPIOName:       pinDef.PinNameCVM,
			PWMSysFsDir:    pwmChipInfo.Dir,
			PWMID:          pinDef.PWMID,
			HWPWMSupported: pinDef.PWMID != -1,
		}
	}
	return data, nil
}
