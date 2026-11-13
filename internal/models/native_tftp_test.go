package models

import "testing"

func TestValidateNativeTFTPBootFile(t *testing.T) {
	for _, name := range []string{"", "ec84083c/kernel7.img", "pi/custom.bin", "bootloader-sets/rpi/ec84083c/kernel7.img"} {
		if err := ValidateNativeTFTPBootFile(name); err != nil {
			t.Errorf("valid %q: %v", name, err)
		}
	}
	for _, name := range []string{"kernel7.img", "/ec84083c/kernel7.img", "../pi/kernel", "pi/../kernel", "pi//kernel", "pi/./kernel", "pi/", `pi\kernel.img`, " pi/kernel.img", "pi/kernel\x00.img", "pi/\xffkernel"} {
		if err := ValidateNativeTFTPBootFile(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}
