package pinp

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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
		GraphDriverName string         `json:"graphDriverName"`
		GraphOptions    map[string]any `json:"graphOptions"`
		GraphRoot       string         `json:"graphRoot"`
		RunRoot         string         `json:"runRoot"`
		TransientStore  bool           `json:"transientStore"`
	} `json:"store"`
}

// AdditionalImageStores returns the additional image stores podman reports as
// actually loaded, read from the effective graphOptions (podman exposes
// "*.additionalImageStores" as a list and the legacy "*.imagestore" as a
// string). This verifies the storage.conf drop-in was really applied, not just
// that the file exists.
func (i *Info) AdditionalImageStores() []string {
	if i == nil || i.Store.GraphOptions == nil {
		return nil
	}
	var out []string
	for key, val := range i.Store.GraphOptions {
		switch {
		case strings.HasSuffix(key, "additionalImageStores"):
			list, ok := val.([]any)
			if !ok {
				continue
			}
			for _, item := range list {
				if store, ok := item.(string); ok {
					out = append(out, store)
				}
			}
		case strings.HasSuffix(key, "imagestore"):
			if store, ok := val.(string); ok {
				out = append(out, store)
			}
		}
	}
	return out
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
