package computeutil

import (
	"fmt"
	"strings"
	"time"

	raw "code.google.com/p/google-api-go-client/compute/v1"
	"golang.org/x/net/context"
	"google.golang.org/cloud/internal"
)

const (
	// apiPrefix define the URL prefix for Compute API resources.
	apiPrefix = "https://www.googleapis.com/compute/v1"
	// ScopeCompute grants permissions to view and manage Compute Engine instance.
	ScopeCompute            = "https://www.googleapis.com/auth/compute"
	instanceInsertOpTick    = 1 * time.Second
	instanceInsertOpTimeout = 5 * time.Minute
)

func rawService(ctx context.Context) (*raw.Service, string, string) {
	base := ctx.Value(internal.ContextKey("base")).(map[string]interface{})
	service := base["compute_service"].(*raw.Service)
	project := base["project_id"].(string)
	zone := ctx.Value(internal.ContextKey("zone")).(string)
	return service, project, zone
}

type Instance struct {
	Name        string
	MachineType string
	Image       string
	Zone        string
	raw.Instance
}

func imageURL(project, image string) string {
	if strings.HasPrefix(image, apiPrefix) {
		// already fully qualifed
		return image
	}
	if strings.HasPrefix(image, "projects/") {
		// only add api prefix.
		return apiPrefix + image
	}
	// fallback on project images.
	return globalResource(project, "images/"+image)
}

func zoneResource(project, zone, resource string) string {
	return fmt.Sprintf("%s/projects/%s/zones/%s/%s", apiPrefix, project, zone, resource)
}

func globalResource(project, resource string) string {
	return fmt.Sprintf("%s/projects/%s/global/%s", apiPrefix, project, resource)
}

// Instance or creates a new instance.
func NewInstance(ctx context.Context, instance *Instance) (*Instance, error) {
	service, project, zone := rawService(ctx)
	if instance.Name == "" {
		return nil, fmt.Errorf("NewInstance: instance Name is required")
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
	op, err := service.Instances.Insert(project, instance.Zone, &instance.Instance).Do()
	if err != nil {
		return nil, fmt.Errorf("instance insert api call failed: %v", err)
	}
	if err := waitOperation(service, project, zone, op, time.Tick(instanceInsertOpTick), time.After(instanceInsertOpTimeout)); err != nil {
		return nil, fmt.Errorf("instance insert operation failed: %v", err)
	}
	instance.SelfLink = op.TargetLink
	return instance, nil
}

// waitOperation waits for an zone operations to be DONE.
func waitOperation(service *raw.Service, project, zone string, operation *raw.Operation, tick, timeout <-chan time.Time) error {
	for {
		select {
		case <-tick:
			op, err := service.ZoneOperations.Get(project, zone, operation.Name).Do()
			if err != nil {
				return fmt.Errorf("failed to get operation %q: %v", operation.Name, err)
			}
			if op.Status == "DONE" {
				if op.Error != nil {
					return fmt.Errorf("operation error: %v", *op.Error.Errors[0])
				}
				return nil
			}
		case <-timeout:
			return fmt.Errorf("waitOperation timeout %q: %v", operation.Name, operation)
		}
	}
}
