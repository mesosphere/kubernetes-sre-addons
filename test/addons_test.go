package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/blang/semver"
	volumetypes "github.com/docker/docker/api/types/volume"
	docker "github.com/docker/docker/client"
	"github.com/google/uuid"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha3"
	"sigs.k8s.io/kind/pkg/cluster"

	"github.com/mesosphere/kubeaddons/hack/temp"
	"github.com/mesosphere/kubeaddons/pkg/api/v1beta1"
	"github.com/mesosphere/kubeaddons/pkg/catalog"
	"github.com/mesosphere/kubeaddons/pkg/repositories"
	"github.com/mesosphere/kubeaddons/pkg/repositories/git"
	"github.com/mesosphere/kubeaddons/pkg/repositories/local"
	"github.com/mesosphere/kubeaddons/pkg/test"
	"github.com/mesosphere/kubeaddons/pkg/test/cluster/kind"
)

const (
	controllerBundle         = "https://mesosphere.github.io/kubeaddons/bundle.yaml"
	defaultKubernetesVersion = "1.15.6"
	patchStorageClass        = `{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"false"}}}`

	comRepoURL    = "https://github.com/mesosphere/kubeaddons-community"
	comRepoRef    = "master"
	comRepoRemote = "origin"
)

var (
	cat       catalog.Catalog
	localRepo repositories.Repository
	comRepo   repositories.Repository
	groups    map[string][]v1beta1.AddonInterface
)

func init() {
	var err error

	localRepo, err = local.NewRepository("local", "../addons")
	if err != nil {
		panic(err)
	}

	comRepo, err = git.NewRemoteRepository(comRepoURL, comRepoRef, comRepoRemote)
	if err != nil {
		panic(err)
	}

	cat, err = catalog.NewCatalog(localRepo, comRepo)
	if err != nil {
		panic(err)
	}

	groups, err = test.AddonsForGroupsFile("groups.yaml", cat)
	if err != nil {
		panic(err)
	}
}

func TestValidateUnhandledAddons(t *testing.T) {
	unhandled, err := findUnhandled()
	if err != nil {
		t.Fatal(err)
	}

	if len(unhandled) != 0 {
		names := make([]string, len(unhandled))
		for _, addon := range unhandled {
			names = append(names, addon.GetName())
		}
		t.Fatal(fmt.Errorf("the following addons are not handled as part of a testing group: %+v", names))
	}
}

// func TestGeneralGroup(t *testing.T) {
// 	if err := testgroup(t, "general"); err != nil {
// 		t.Fatal(err)
// 	}
// }
//
// func TestBackupsGroup(t *testing.T) {
// 	if err := testgroup(t, "backups"); err != nil {
// 		t.Fatal(err)
// 	}
// }
//
// func TestSsoGroup(t *testing.T) {
// 	if err := testgroup(t, "sso"); err != nil {
// 		t.Fatal(err)
// 	}
// }
//
// func TestElasticsearchGroup(t *testing.T) {
// 	if err := testgroup(t, "elasticsearch"); err != nil {
// 		t.Fatal(err)
// 	}
// }
//
// func TestPrometheusGroup(t *testing.T) {
// 	if err := testgroup(t, "prometheus"); err != nil {
// 		t.Fatal(err)
// 	}
// }
//
// func TestIstioGroup(t *testing.T) {
// 	if err := testgroup(t, "istio"); err != nil {
// 		t.Fatal(err)
// 	}
// }
//
// func TestLocalVolumeProvisionerGroup(t *testing.T) {
// 	if err := testgroup(t, "localvolumeprovisioner"); err != nil {
// 		t.Fatal(err)
// 	}
// }

func TestKiamGroup(t *testing.T) {
	if err := testgroup(t, "kiam"); err != nil {
		t.Fatal(err)
	}
}

// -----------------------------------------------------------------------------
// Private Functions
// -----------------------------------------------------------------------------

func createNodeVolumes(numberVolumes int, nodePrefix string, node *v1alpha3.Node) error {
	dockerClient, err := docker.NewClientWithOpts(docker.FromEnv)
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	dockerClient.NegotiateAPIVersion(context.TODO())

	for index := 0; index < numberVolumes; index++ {
		volumeName := fmt.Sprintf("%s-%d", nodePrefix, index)

		volume, err := dockerClient.VolumeCreate(context.TODO(), volumetypes.VolumeCreateBody{
			Driver: "local",
			Name:   volumeName,
		})
		if err != nil {
			return fmt.Errorf("creating volume for node: %w", err)
		}

		node.ExtraMounts = append(node.ExtraMounts, v1alpha3.Mount{
			ContainerPath: fmt.Sprintf("/mnt/disks/%s", volumeName),
			HostPath:      volume.Mountpoint,
		})
	}

	return nil
}

func cleanupNodeVolumes(numberVolumes int, nodePrefix string, node *v1alpha3.Node) error {
	dockerClient, err := docker.NewClientWithOpts(docker.FromEnv)
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	dockerClient.NegotiateAPIVersion(context.TODO())

	for index := 0; index < numberVolumes; index++ {
		volumeName := fmt.Sprintf("%s-%d", nodePrefix, index)

		if err := dockerClient.VolumeRemove(context.TODO(), volumeName, false); err != nil {
			return fmt.Errorf("removing volume for node: %w", err)
		}
	}

	return nil
}

func testgroup(t *testing.T, groupname string) error {
	t.Logf("testing group %s", groupname)

	version, err := semver.Parse(defaultKubernetesVersion)
	if err != nil {
		return err
	}

	u := uuid.New()

	node := v1alpha3.Node{}
	if err := createNodeVolumes(3, u.String(), &node); err != nil {
		return err
	}
	defer func() {
		if err := cleanupNodeVolumes(3, u.String(), &node); err != nil {
			t.Logf("error: %s", err)
		}
	}()

	cluster, err := kind.NewCluster(version, cluster.CreateWithV1Alpha3Config(&v1alpha3.Cluster{
		Nodes: []v1alpha3.Node{node},
	}))
	if err != nil {
		// try to clean up in case cluster was created and reference available
		if cluster != nil {
			_ = cluster.Cleanup()
		}
		return err
	}
	defer cluster.Cleanup()

	if err := kubectl("apply", "-f", controllerBundle); err != nil {
		return err
	}

	addons := groups[groupname]
	for _, addon := range addons {
		overrides(addon)
	}

	ph, err := test.NewBasicTestHarness(t, cluster, addons...)
	if err != nil {
		return err
	}
	defer ph.Cleanup()

	wg := &sync.WaitGroup{}
	stop := make(chan struct{})
	go temp.LoggingHook(t, cluster, wg, stop)

	ph.Validate()
	ph.Deploy()

	close(stop)
	wg.Wait()

	return nil
}

func findUnhandled() ([]v1beta1.AddonInterface, error) {
	var unhandled []v1beta1.AddonInterface
	repo, err := local.NewRepository("base", "../addons")
	if err != nil {
		return unhandled, err
	}
	addons, err := repo.ListAddons()
	if err != nil {
		return unhandled, err
	}

	for _, revisions := range addons {
		addon := revisions[0]
		found := false
		for _, v := range groups {
			for _, r := range v {
				if r.GetName() == addon.GetName() {
					found = true
				}
			}
		}
		if !found {
			unhandled = append(unhandled, addon)
		}
	}

	return unhandled, nil
}

func removeLocalPathAsDefaultStorage(cluster test.Cluster, addons []v1beta1.AddonInterface) error {
	for _, addon := range addons {
		if addon.GetName() == "localvolumeprovisioner" {
			if err := kubectl("patch", "storageclass", "local-path", "-p", patchStorageClass); err != nil {
				return err
			}
			return nil
		}
	}
	return nil
}

func kubectl(args ...string) error {
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// -----------------------------------------------------------------------------
// Private - CI Values Overrides
// -----------------------------------------------------------------------------

// TODO: a temporary place to put configuration overrides for addons
// See: https://jira.mesosphere.com/browse/DCOS-62137
func overrides(addon v1beta1.AddonInterface) {
	if v, ok := addonOverrides[addon.GetName()]; ok {
		addon.GetAddonSpec().ChartReference.Values = &v
	}
}

var addonOverrides = map[string]string{
	"metallb": `
---
configInline:
  address-pools:
  - name: default
    protocol: layer2
    addresses:
    - "172.17.1.200-172.17.1.250"
`,
}
