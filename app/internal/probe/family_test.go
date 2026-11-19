package probe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/birkskyum/arewearmyet/app/internal/machine"
)

func TestTheFamilyIsReadFromWhatTheMachineSaysOfItself(t *testing.T) {
	family := func(files map[string]string) string {
		root := t.TempDir()
		for path, text := range files {
			os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
			os.WriteFile(filepath.Join(root, path), []byte(text), 0o644)
		}
		return Family(machine.Fixture(root))
	}
	tree := "proc/device-tree/compatible"
	for name, test := range map[string]struct {
		files map[string]string
		want  string
	}{
		"a Snapdragon laptop that came with Windows": {map[string]string{tree: "lenovo,yoga-slim7x\x00qcom,x1e80100\x00"}, DragonMsft},
		"a Snapdragon Chromebook":                    {map[string]string{tree: "google,lazor-rev1\x00qcom,sc7180\x00"}, DragonGoogle},
		"a Googlebook":                               {map[string]string{tree: "google,mica\x00qcom,x1e80100\x00"}, DragonGoogle},
		"an NVIDIA machine with a tree":              {map[string]string{tree: "nvidia,p3971-0089\x00nvidia,tegra264\x00"}, Nvidia},
		"an NVIDIA machine started without a tree":   {map[string]string{"sys/class/dmi/id/sys_vendor": "NVIDIA\n", "sys/class/dmi/id/product_name": "NVIDIA_DGX_Spark\n", "proc/cpuinfo": "CPU implementer\t: 0x41\n"}, Nvidia},
		"a Mac": {map[string]string{tree: "apple,j314s\x00apple,t6000\x00apple,arm-platform\x00"}, Apple},
		"a Snapdragon machine started without a tree": {map[string]string{"sys/class/dmi/id/sys_vendor": "LENOVO\n", "proc/cpuinfo": "processor\t: 0\nCPU implementer\t: 0x51\n"}, DragonMsft},
		// A Google board with another maker's chip isn't a Snapdragon one
		"a Chromebook with another chip": {map[string]string{tree: "google,tomato\x00mediatek,mt8195\x00"}, ""},
		"nothing that tells":             {map[string]string{"proc/cpuinfo": "CPU implementer\t: 0x41\n"}, ""},
	} {
		if got := family(test.files); got != test.want {
			t.Errorf("%s: %q, want %q", name, got, test.want)
		}
	}
}
