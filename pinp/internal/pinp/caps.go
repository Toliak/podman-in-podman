package pinp

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/syndtr/gocapability/capability"
)

func getcapStatus(path, label string) (bool, string) {
	out, err := exec.Command("getcap", path).Output()
	if err != nil {
		return false, fmt.Sprintf("getcap failed: %v", err)
	}
	for _, field := range strings.Fields(string(out)) {
		if !strings.HasPrefix(field, label+"=") {
			continue
		}
		flags := strings.TrimPrefix(field, label+"=")
		if strings.Contains(flags, "e") && strings.Contains(flags, "p") {
			return true, label + "=ep"
		}
		return false, label + "=" + flags
	}
	return false, "capability not reported by getcap"
}

func FileCapStatus(path string, want capability.Cap, label string) (bool, string) {
	c, err := capability.NewFile(path)
	if err == nil {
		if !c.Get(capability.PERMITTED, want) {
			return false, label + " missing from permitted set"
		}
		if !c.Get(capability.EFFECTIVE, want) {
			return false, label + " not effective"
		}
		return true, label + "=ep"
	}
	ok, detail := getcapStatus(path, label)
	if ok {
		return true, detail
	}
	return false, fmt.Sprintf("no file capabilities (%v); getcap: %s", err, detail)
}

func IsSetuid(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSetuid != 0, nil
}
