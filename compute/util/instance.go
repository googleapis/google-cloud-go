package computeutil

import (
	"fmt"
	"time"

	raw "code.google.com/p/google-api-go-client/compute/v1"
	"golang.org/x/net/context"
)

const (
	instanceInsertOpTick    = 1 * time.Second
	instanceInsertOpTimeout = 5 * time.Minute
)

type Instance struct {
	Name        string
	MachineType string
	Image       string
	Zone        string
	*raw.Instance
}

// NewInstance creates a new instance.
func NewInstance(ctx context.Context, instance *Instance) (*Instance, error) {
	service, project, zone := rawService(ctx)
	if instance.Name == "" {
		return nil, fmt.Errorf("NewInstance: instance Name is required")
	}
	if instance.Instance == nil {
		instance.Instance = &raw.Instance{}
	}
	instance.Instance.Name = instance.Name
	if instance.Zone == "" {
		instance.Zone = zone
	}
	if instance.MachineType != "" {
		instance.Instance.MachineType = zoneResource(project, zone, "machineTypes/"+instance.MachineType)
	}
	if instance.Image != "" {
		instance.Disks = []*raw.AttachedDisk{
			{
				Boot:       true,
				AutoDelete: true,
				Type:       "PERSISTENT",
				InitializeParams: &raw.AttachedDiskInitializeParams{
					DiskName:    instance.Name + "-root-disk",
					SourceImage: instance.Image,
					DiskType:    zoneResource(project, zone, "diskTypes/pd-standard"),
				},
			},
		}
	}
	if instance.NetworkInterfaces == nil {
		instance.NetworkInterfaces = []*raw.NetworkInterface{
			{
				AccessConfigs: []*raw.AccessConfig{
					&raw.AccessConfig{Type: "ONE_TO_ONE_NAT"},
				},
				Network: globalResource(project, "networks/default"),
			},
		}
	}
	op, err := service.Instances.Insert(project, instance.Zone, instance.Instance).Do()
	if err != nil {
		return nil, fmt.Errorf("instance insert api call failed: %v", err)
	}
	if err := waitOperation(service, project, zone, op, time.Tick(instanceInsertOpTick), time.After(instanceInsertOpTimeout)); err != nil {
		return nil, fmt.Errorf("instance insert operation failed: %v", err)
	}
	instance.SelfLink = op.TargetLink
	return instance, nil
}

// GetInstance gets an existing instance resource.
func GetInstance(ctx context.Context, name string) (*Instance, error) {
	service, project, zone := rawService(ctx)
	if name == "" {
		return nil, fmt.Errorf("GetInstance: instance name is required")
	}
	instance, err := service.Instances.Get(project, zone, name).Do()
	if err != nil {
		return nil, fmt.Errorf("GetInstance: failed to get instance %q: %v", name, err)
	}
	return &Instance{
		Name:        instance.Name,
		MachineType: resourceName(instance.MachineType),
		Zone:        resourceName(instance.Zone),
		Instance:    instance,
	}, nil
}
