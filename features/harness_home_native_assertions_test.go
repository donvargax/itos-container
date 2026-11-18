package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

func homeNativeArray(value any) []any {
	items, _ := value.([]any)
	return items
}

func (w *world) homeNativeMounts() error {
	f := w.home.native
	if len(f.volumes) != 1 || len(f.workers) != 2 || len(f.anchors) != 2 {
		return fmt.Errorf("expected one retained home, two distinct workers and two checkout anchors; got %d/%d/%d", len(f.volumes), len(f.workers), len(f.anchors))
	}
	for _, run := range f.runs {
		if run.report.HostReadable {
			return errors.New("home worker read an isolated host-home sentinel")
		}
		if run.home == nil || run.home["Driver"] != "local" || run.home["Scope"] != "local" || len(object(run.home["Options"])) != 0 || run.home["CreatedAt"] == "" {
			return errors.New("home worker did not use a recorded plain local volume")
		}
		host, config := object(run.worker["HostConfig"]), object(run.worker["Config"])
		if config["User"] != w.approval.uid+":"+w.approval.gid || host["ReadonlyRootfs"] != true || host["NetworkMode"] != "none" || host["Privileged"] == true || host["AutoRemove"] == true || host["PublishAllPorts"] == true ||
			len(homeNativeArray(host["Binds"])) != 0 || len(object(config["Volumes"])) != 0 || len(homeNativeArray(host["CapAdd"])) != 0 || len(homeNativeArray(host["GroupAdd"])) != 0 ||
			!reflect.DeepEqual(host["CapDrop"], []any{"ALL"}) || !reflect.DeepEqual(host["SecurityOpt"], []any{"no-new-privileges"}) {
			return errors.New("home worker lost the hardened non-root boundary")
		}
		env := map[string]string{}
		for _, value := range homeNativeArray(config["Env"]) {
			key, data, _ := strings.Cut(fmt.Sprint(value), "=")
			env[key] = data
		}
		for key, value := range map[string]string{"HOME": "/home/itos", "TMPDIR": "/tmp", "XDG_CONFIG_HOME": "/home/itos/.config", "XDG_DATA_HOME": "/home/itos/.local/share", "XDG_STATE_HOME": "/home/itos/.local/state", "XDG_CACHE_HOME": "/home/itos/.cache"} {
			if env[key] != value {
				return fmt.Errorf("home worker environment %s differs from its approved mapping", key)
			}
		}
		if !reflect.DeepEqual(host["Tmpfs"], map[string]any{"/tmp": "rw,exec,nosuid,nodev,mode=1777"}) {
			return errors.New("home worker has an unexpected temporary mount")
		}
		requested := homeNativeArray(host["Mounts"])
		if len(requested) != 2 {
			return errors.New("home worker has extra or missing requested mounts")
		}
		byTarget := map[string]map[string]any{}
		for _, value := range requested {
			mount := object(value)
			target, _ := mount["Target"].(string)
			if byTarget[target] != nil || mount["Type"] != "volume" || mount["ReadOnly"] == true || object(mount["VolumeOptions"])["NoCopy"] != true ||
				mount["BindOptions"] != nil || mount["TmpfsOptions"] != nil || object(mount["VolumeOptions"])["DriverConfig"] != nil ||
				len(object(object(mount["VolumeOptions"])["Labels"])) != 0 {
				return errors.New("home worker requested an unapproved mount or image-home copying")
			}
			if subpath := object(mount["VolumeOptions"])["Subpath"]; subpath != nil && subpath != "" {
				return errors.New("home worker requested a volume subpath")
			}
			byTarget[target] = mount
		}
		if byTarget["/home/itos"]["Source"] != run.home["Name"] || f.anchors[fmt.Sprint(byTarget["/workspace"]["Source"])] == nil {
			return errors.New("home worker requested another volume or checkout anchor")
		}
		seen := map[string]bool{}
		for _, value := range homeNativeArray(run.worker["Mounts"]) {
			mount := object(value)
			target, _ := mount["Destination"].(string)
			if seen[target] {
				return errors.New("home worker has overlapping mounts")
			}
			seen[target] = true
			if target == "/tmp" && mount["Type"] == "tmpfs" {
				continue
			}
			expected := byTarget[target]
			if expected == nil || mount["Type"] != "volume" || mount["Driver"] != "local" || mount["Name"] != expected["Source"] || mount["RW"] != true {
				return errors.New("home worker actually mounted an unapproved filesystem")
			}
			volume := f.volumes[fmt.Sprint(mount["Name"])]
			if target == "/workspace" {
				volume = f.anchors[fmt.Sprint(mount["Name"])]
				options := object(volume["Options"])
				if options["type"] != "none" || options["o"] != "bind" || len(options) != 3 || !strings.HasPrefix(fmt.Sprint(options["device"]), "/proc/") {
					return errors.New("checkout mount lost its pinned anchor")
				}
			}
			if mount["Source"] != volume["Mountpoint"] {
				return errors.New("home worker mount source differs from native volume inspection")
			}
		}
		if !seen["/home/itos"] || !seen["/workspace"] {
			return errors.New("home worker lacks its approved native mounts")
		}
	}
	if !homeNativeSameVolume(f.runs[0].home, f.runs[1].home) {
		return errors.New("sequential runs did not reuse the same concrete home")
	}
	return nil
}

func (w *world) homeNativeResources() error {
	return w.homeNativeResourcesWith(w.home.native.volumes)
}

func (w *world) homeNativeResourcesWith(homes map[string]map[string]any) error {
	f := w.home.native
	for id := range f.workers {
		if err := w.doctor.real.read("/containers/"+id+"/json", nil, 404); err != nil {
			return fmt.Errorf("home worker remains before harness teardown: %w", err)
		}
	}
	for name := range f.anchors {
		if err := w.doctor.real.read("/volumes/"+name, nil, 404); err != nil {
			return fmt.Errorf("home checkout anchor remains before harness teardown: %w", err)
		}
	}
	for name, expected := range f.volumes {
		var observed map[string]any
		if err := w.doctor.real.read("/volumes/"+name, &observed, 200); err != nil {
			return err
		}
		if !homeNativeSameVolume(expected, observed) {
			return errors.New("retained home changed its native identity")
		}
	}
	resources, err := w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	// Remove only the exact, independently observed retained home additions from
	// the resource comparison. Every pre-existing volume remains in the audit.
	var retained []string
	for _, line := range strings.Split(resources["/volumes"], "\n") {
		if line == "" {
			continue
		}
		var volume map[string]any
		if err := json.Unmarshal([]byte(line), &volume); err != nil {
			return err
		}
		if homes[fmt.Sprint(volume["Name"])] == nil {
			retained = append(retained, line)
		}
	}
	slices.Sort(retained)
	resources["/volumes"] = strings.Join(retained, "\n")
	if !reflect.DeepEqual(f.resourcesBefore, resources) {
		return errors.New("home runs changed unrelated native resources or leaked session resources")
	}
	return nil
}
