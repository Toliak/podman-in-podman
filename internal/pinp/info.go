package pinp

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

type IDMapEntry struct {
	ContainerID int `json:"container_id"`
	HostID      int `json:"host_id"`
	Size        int `json:"size"`
}

type Info struct {
	Host struct {
		Arch               string `json:"arch"`
		BuildahVersion     string `json:"buildahVersion"`
		CgroupManager      string `json:"cgroupManager"`
		CgroupsVersion     string `json:"cgroupVersion"`
		NetworkBackend     string `json:"networkBackend"`
		ServiceIsRemote    bool   `json:"serviceIsRemote"`
		RootlessNetworkCmd string `json:"rootlessNetworkCmd"`
		IDMappings         struct {
			UIDMap []IDMapEntry `json:"uidmap"`
			GIDMap []IDMapEntry `json:"gidmap"`
		} `json:"idMappings"`
		OCIRuntime struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"ociRuntime"`
		Security struct {
			AppArmorEnabled     bool   `json:"apparmorEnabled"`
			DefaultCapabilities string `json:"capabilities"`
			Rootless            bool   `json:"rootless"`
			SECCOMPEnabled      bool   `json:"seccompEnabled"`
			SELinuxEnabled      bool   `json:"selinuxEnabled"`
		} `json:"security"`
		Pasta struct {
			Executable string `json:"executable"`
		} `json:"pasta"`
	} `json:"host"`
	Store struct {
		GraphDriverName string `json:"graphDriverName"`
		GraphRoot       string `json:"graphRoot"`
		RunRoot         string `json:"runRoot"`
		TransientStore  bool   `json:"transientStore"`
	} `json:"store"`
}

func GetPodmanInfo(podman string) (*Info, error) {
	out, err := exec.Command(podman, "info", "--format", "{{json .}}").Output()
	if err != nil {
		return nil, fmt.Errorf("podman info: %w", err)
	}
	var info Info
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("decode podman info: %w", err)
	}
	return &info, nil
}
