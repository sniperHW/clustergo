// +build darwin openbsd

package proc

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cjbassi/gotop/src/utils"
)

const (
	// Define column widths for ps output used in Procs()
	five   = "12345"
	twenty = ten + ten
	ten    = five + five
	fifty  = ten + ten + ten + ten + ten
)

func GetProcs(filter ...string) ([]Proc, error) {

	filterFn := createFilter(filter)

	keywords := fmt.Sprintf("user=%s,pid=%s,comm=%s,pcpu=%s,pmem=%s,args", twenty, ten, fifty, five, five)
	output, err := exec.Command("ps", "-caxo", keywords).Output()

	if err != nil {
		return nil, fmt.Errorf("failed to execute 'ps' command: %v", err)
	}

	// converts to []string and removes the header
	linesOfProcStrings := strings.Split(strings.TrimSpace(string(output)), "\n")[1:]

	procs := []Proc{}
	for _, line := range linesOfProcStrings {
		pid, err := strconv.Atoi(strings.TrimSpace(line[21:31]))
		if err != nil {
			log.Printf("failed to convert first field to int: %v. split: %v", err, line)
			continue
		}
		cpu, err := strconv.ParseFloat(utils.ConvertLocalizedString(strings.TrimSpace(line[83:88])), 64)
		if err != nil {
			log.Printf("failed to convert third field to float: %v. split: %v", err, line)
			continue
		}
		mem, err := strconv.ParseFloat(utils.ConvertLocalizedString(strings.TrimSpace(line[89:94])), 64)
		if err != nil {
			log.Printf("failed to convert fourth field to float: %v. split: %v", err, line)
			continue
		}

		if filterFn(line[95:]) {
			proc := Proc{
				User:        strings.TrimSpace(line[0:20]),
				Pid:         pid,
				CommandName: strings.TrimSpace(line[32:82]),
				Cpu:         cpu,
				Mem:         mem,
				FullCommand: strings.TrimSpace(line[95:]),
			}
			procs = append(procs, proc)
		}
	}

	return procs, nil
}
