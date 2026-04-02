package cli

import (
	"sort"
	"strings"
)

type virshVM struct {
	Name  string
	State string
}

func parseVirshList(output string) []virshVM {
	var vms []virshVM
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "Id" || strings.HasPrefix(fields[0], "---") {
			continue
		}
		vms = append(vms, virshVM{
			Name:  fields[1],
			State: strings.Join(fields[2:], " "),
		})
	}

	sort.Slice(vms, func(i, j int) bool {
		if vms[i].Name == vms[j].Name {
			return vms[i].State < vms[j].State
		}
		return vms[i].Name < vms[j].Name
	})
	return vms
}
