package driver

import (
	"context"
	"io"
	"testing"

	proto "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/go-kit/kit/log"
	"github.com/hetznercloud/hcloud-go/hcloud"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"hetzner.cloud/csi/csi"
	"hetzner.cloud/csi/mock"
	"hetzner.cloud/csi/volumes"
)

var _ proto.NodeServer = (*NodeService)(nil)

type nodeServiceTestEnv struct {
	ctx                context.Context
	service            *NodeService
	server             *hcloud.Server
	volumeService      *mock.VolumeService
	volumeMountService *mock.VolumeMountService
}

func newNodeServerTestEnv() nodeServiceTestEnv {
	var (
		server = &hcloud.Server{
			ID: 1,
			Datacenter: &hcloud.Datacenter{
				Location: &hcloud.Location{
					Name: "test",
				},
			},
		}
		volumeService      = &mock.VolumeService{}
		volumeMountService = &mock.VolumeMountService{}
	)
	return nodeServiceTestEnv{
		ctx: context.Background(),
		service: NewNodeService(
			log.NewNopLogger(),
			server,
			volumeService,
			volumeMountService,
		),
		server:             server,
		volumeService:      volumeService,
		volumeMountService: volumeMountService,
	}
}

func TestNodeServiceNodeStageVolume(t *testing.T) {
	env := newNodeServerTestEnv()

	existingVolume := &csi.Volume{}

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		if id != 1 {
			t.Errorf("unexpected volume id passed to volume service: %d", id)
		}
		return existingVolume, nil
	}

	env.volumeMountService.StageFunc = func(volume *csi.Volume, stagingTargetPath string, opts volumes.MountOpts) error {
		if volume != existingVolume {
			t.Errorf("unexpected volume passed to volume mount service: %v", volume)
		}
		if stagingTargetPath != "staging" {
			t.Errorf("unexpected staging target path passed to volume mount service: %s", stagingTargetPath)
		}
		if opts.FSType != "ext4" {
			t.Errorf("unexpected fs type in mount options: %s", opts.FSType)
		}
		if len(opts.Additional) != 2 || opts.Additional[0] != "flag1" || opts.Additional[1] != "flag2" {
			t.Errorf("unexpected additional options in mount options: %v", opts.Additional)
		}
		return nil
	}

	_, err := env.service.NodeStageVolume(env.ctx, &proto.NodeStageVolumeRequest{
		VolumeId:          "1",
		StagingTargetPath: "staging",
		VolumeCapability: &proto.VolumeCapability{
			AccessMode: &proto.VolumeCapability_AccessMode{
				Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
			AccessType: &proto.VolumeCapability_Mount{
				Mount: &proto.VolumeCapability_MountVolume{
					FsType:     "ext4",
					MountFlags: []string{"flag1", "flag2"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeServiceNodeStageVolumeNotFound(t *testing.T) {
	env := newNodeServerTestEnv()

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		return nil, volumes.ErrVolumeNotFound
	}

	_, err := env.service.NodeStageVolume(env.ctx, &proto.NodeStageVolumeRequest{
		VolumeId:          "1",
		StagingTargetPath: "staging",
		VolumeCapability: &proto.VolumeCapability{
			AccessMode: &proto.VolumeCapability_AccessMode{
				Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
			AccessType: &proto.VolumeCapability_Mount{
				Mount: &proto.VolumeCapability_MountVolume{
					FsType:     "ext4",
					MountFlags: []string{"flag1", "flag2"},
				},
			},
		},
	})
	if grpc.Code(err) != codes.NotFound {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNodeServiceNodeStageVolumeStageError(t *testing.T) {
	env := newNodeServerTestEnv()

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		return &csi.Volume{}, nil
	}

	env.volumeMountService.StageFunc = func(volume *csi.Volume, stagingTargetPath string, opts volumes.MountOpts) error {
		return io.EOF
	}

	_, err := env.service.NodeStageVolume(env.ctx, &proto.NodeStageVolumeRequest{
		VolumeId:          "1",
		StagingTargetPath: "staging",
		VolumeCapability: &proto.VolumeCapability{
			AccessMode: &proto.VolumeCapability_AccessMode{
				Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
			AccessType: &proto.VolumeCapability_Mount{
				Mount: &proto.VolumeCapability_MountVolume{
					FsType:     "ext4",
					MountFlags: []string{"flag1", "flag2"},
				},
			},
		},
	})
	if grpc.Code(err) != codes.Internal {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNodeServiceNodeStageVolumeInputErrors(t *testing.T) {
	env := newNodeServerTestEnv()

	testCases := []struct {
		Name string
		Req  *proto.NodeStageVolumeRequest
		Code codes.Code
	}{
		{
			Name: "empty volume id",
			Req: &proto.NodeStageVolumeRequest{
				StagingTargetPath: "staging",
				VolumeCapability: &proto.VolumeCapability{
					AccessMode: &proto.VolumeCapability_AccessMode{
						Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &proto.VolumeCapability_Mount{
						Mount: &proto.VolumeCapability_MountVolume{
							FsType:     "ext4",
							MountFlags: []string{"flag1", "flag2"},
						},
					},
				},
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "empty staging target path",
			Req: &proto.NodeStageVolumeRequest{
				VolumeId: "1",
				VolumeCapability: &proto.VolumeCapability{
					AccessMode: &proto.VolumeCapability_AccessMode{
						Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &proto.VolumeCapability_Mount{
						Mount: &proto.VolumeCapability_MountVolume{
							FsType:     "ext4",
							MountFlags: []string{"flag1", "flag2"},
						},
					},
				},
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "empty volume capability",
			Req: &proto.NodeStageVolumeRequest{
				VolumeId:          "1",
				StagingTargetPath: "staging",
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "no mount access type",
			Req: &proto.NodeStageVolumeRequest{
				VolumeId:          "1",
				StagingTargetPath: "staging",
				VolumeCapability: &proto.VolumeCapability{
					AccessMode: &proto.VolumeCapability_AccessMode{
						Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "invalid volume id",
			Req: &proto.NodeStageVolumeRequest{
				VolumeId:          "xxx",
				StagingTargetPath: "staging",
				VolumeCapability: &proto.VolumeCapability{
					AccessMode: &proto.VolumeCapability_AccessMode{
						Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &proto.VolumeCapability_Mount{
						Mount: &proto.VolumeCapability_MountVolume{
							FsType:     "ext4",
							MountFlags: []string{"flag1", "flag2"},
						},
					},
				},
			},
			Code: codes.NotFound,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			_, err := env.service.NodeStageVolume(env.ctx, testCase.Req)
			if grpc.Code(err) != testCase.Code {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}

func TestNodeServiceNodeUnstageVolume(t *testing.T) {
	env := newNodeServerTestEnv()

	existingVolume := &csi.Volume{}

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		if id != 1 {
			t.Errorf("unexpected volume id passed to volume service: %d", id)
		}
		return existingVolume, nil
	}

	env.volumeMountService.UnstageFunc = func(volume *csi.Volume, stagingTargetPath string) error {
		if volume != existingVolume {
			t.Errorf("unexpected volume passed to volume mount service: %v", volume)
		}
		if stagingTargetPath != "staging" {
			t.Errorf("unexpected staging target path passed to volume mount service: %s", stagingTargetPath)
		}
		return nil
	}

	_, err := env.service.NodeUnstageVolume(env.ctx, &proto.NodeUnstageVolumeRequest{
		VolumeId:          "1",
		StagingTargetPath: "staging",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeServiceNodeUnstageVolumeNotFound(t *testing.T) {
	env := newNodeServerTestEnv()

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		return nil, volumes.ErrVolumeNotFound
	}

	_, err := env.service.NodeUnstageVolume(env.ctx, &proto.NodeUnstageVolumeRequest{
		VolumeId:          "1",
		StagingTargetPath: "staging",
	})
	if grpc.Code(err) != codes.NotFound {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNodeServiceNodeUnstageVolumeUnstageError(t *testing.T) {
	env := newNodeServerTestEnv()

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		return &csi.Volume{}, nil
	}

	env.volumeMountService.UnstageFunc = func(volume *csi.Volume, stagingTargetPath string) error {
		return io.EOF
	}

	_, err := env.service.NodeUnstageVolume(env.ctx, &proto.NodeUnstageVolumeRequest{
		VolumeId:          "1",
		StagingTargetPath: "staging",
	})
	if grpc.Code(err) != codes.Internal {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNodeServiceNodeUnstageVolumeInputErrors(t *testing.T) {
	env := newNodeServerTestEnv()

	testCases := []struct {
		Name string
		Req  *proto.NodeUnstageVolumeRequest
		Code codes.Code
	}{
		{
			Name: "empty volume id",
			Req: &proto.NodeUnstageVolumeRequest{
				StagingTargetPath: "staging",
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "empty staging target path",
			Req: &proto.NodeUnstageVolumeRequest{
				VolumeId: "1",
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "invalid volume id",
			Req: &proto.NodeUnstageVolumeRequest{
				VolumeId:          "xxx",
				StagingTargetPath: "staging",
			},
			Code: codes.NotFound,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			_, err := env.service.NodeUnstageVolume(env.ctx, testCase.Req)
			if grpc.Code(err) != testCase.Code {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}

func TestNodeServiceNodePublishVolume(t *testing.T) {
	env := newNodeServerTestEnv()

	existingVolume := &csi.Volume{}

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		if id != 1 {
			t.Errorf("unexpected volume id passed to volume service: %d", id)
		}
		return existingVolume, nil
	}

	env.volumeMountService.PublishFunc = func(volume *csi.Volume, targetPath string, stagingTargetPath string, opts volumes.MountOpts) error {
		if volume != existingVolume {
			t.Errorf("unexpected volume passed to volume mount service: %v", volume)
		}
		if targetPath != "target" {
			t.Errorf("unexpected target path passed to volume service: %s", targetPath)
		}
		if stagingTargetPath != "staging" {
			t.Errorf("unexpected staging target path passed to volume mount service: %s", stagingTargetPath)
		}
		return nil
	}

	_, err := env.service.NodePublishVolume(env.ctx, &proto.NodePublishVolumeRequest{
		VolumeId:          "1",
		TargetPath:        "target",
		StagingTargetPath: "staging",
		VolumeCapability: &proto.VolumeCapability{
			AccessMode: &proto.VolumeCapability_AccessMode{
				Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
			AccessType: &proto.VolumeCapability_Mount{
				Mount: &proto.VolumeCapability_MountVolume{
					FsType:     "ext4",
					MountFlags: []string{"flag1", "flag2"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeServiceNodePublishVolumeNotFound(t *testing.T) {
	env := newNodeServerTestEnv()

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		return nil, volumes.ErrVolumeNotFound
	}

	_, err := env.service.NodePublishVolume(env.ctx, &proto.NodePublishVolumeRequest{
		VolumeId:          "1",
		TargetPath:        "target",
		StagingTargetPath: "staging",
		VolumeCapability: &proto.VolumeCapability{
			AccessMode: &proto.VolumeCapability_AccessMode{
				Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
			AccessType: &proto.VolumeCapability_Mount{
				Mount: &proto.VolumeCapability_MountVolume{
					FsType:     "ext4",
					MountFlags: []string{"flag1", "flag2"},
				},
			},
		},
	})
	if grpc.Code(err) != codes.NotFound {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNodeServiceNodePublishPublishError(t *testing.T) {
	env := newNodeServerTestEnv()

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		return &csi.Volume{}, nil
	}

	env.volumeMountService.PublishFunc = func(volume *csi.Volume, targetPath string, stagingTargetPath string, opts volumes.MountOpts) error {
		return io.EOF
	}

	_, err := env.service.NodePublishVolume(env.ctx, &proto.NodePublishVolumeRequest{
		VolumeId:          "1",
		TargetPath:        "target",
		StagingTargetPath: "staging",
		VolumeCapability: &proto.VolumeCapability{
			AccessMode: &proto.VolumeCapability_AccessMode{
				Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
			AccessType: &proto.VolumeCapability_Mount{
				Mount: &proto.VolumeCapability_MountVolume{
					FsType:     "ext4",
					MountFlags: []string{"flag1", "flag2"},
				},
			},
		},
	})
	if grpc.Code(err) != codes.Internal {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNodeServiceNodePublishVolumeInputErrors(t *testing.T) {
	env := newNodeServerTestEnv()

	testCases := []struct {
		Name string
		Req  *proto.NodePublishVolumeRequest
		Code codes.Code
	}{
		{
			Name: "empty volume id",
			Req: &proto.NodePublishVolumeRequest{
				TargetPath:        "target",
				StagingTargetPath: "staging",
				VolumeCapability: &proto.VolumeCapability{
					AccessMode: &proto.VolumeCapability_AccessMode{
						Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &proto.VolumeCapability_Mount{
						Mount: &proto.VolumeCapability_MountVolume{
							FsType:     "ext4",
							MountFlags: []string{"flag1", "flag2"},
						},
					},
				},
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "empty target path",
			Req: &proto.NodePublishVolumeRequest{
				VolumeId:          "1",
				StagingTargetPath: "staging",
				VolumeCapability: &proto.VolumeCapability{
					AccessMode: &proto.VolumeCapability_AccessMode{
						Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &proto.VolumeCapability_Mount{
						Mount: &proto.VolumeCapability_MountVolume{
							FsType:     "ext4",
							MountFlags: []string{"flag1", "flag2"},
						},
					},
				},
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "empty staging target path",
			Req: &proto.NodePublishVolumeRequest{
				VolumeId:   "1",
				TargetPath: "target",
				VolumeCapability: &proto.VolumeCapability{
					AccessMode: &proto.VolumeCapability_AccessMode{
						Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &proto.VolumeCapability_Mount{
						Mount: &proto.VolumeCapability_MountVolume{
							FsType:     "ext4",
							MountFlags: []string{"flag1", "flag2"},
						},
					},
				},
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "invalid volume id",
			Req: &proto.NodePublishVolumeRequest{
				VolumeId:          "xxx",
				TargetPath:        "target",
				StagingTargetPath: "staging",
				VolumeCapability: &proto.VolumeCapability{
					AccessMode: &proto.VolumeCapability_AccessMode{
						Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &proto.VolumeCapability_Mount{
						Mount: &proto.VolumeCapability_MountVolume{
							FsType:     "ext4",
							MountFlags: []string{"flag1", "flag2"},
						},
					},
				},
			},
			Code: codes.NotFound,
		},
		{
			Name: "no mount access type",
			Req: &proto.NodePublishVolumeRequest{
				VolumeId:          "1",
				TargetPath:        "target",
				StagingTargetPath: "staging",
				VolumeCapability: &proto.VolumeCapability{
					AccessMode: &proto.VolumeCapability_AccessMode{
						Mode: proto.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			Code: codes.InvalidArgument,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			_, err := env.service.NodePublishVolume(env.ctx, testCase.Req)
			if grpc.Code(err) != testCase.Code {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}

func TestNodeServiceNodeUnpublishVolume(t *testing.T) {
	env := newNodeServerTestEnv()

	existingVolume := &csi.Volume{}

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		if id != 1 {
			t.Errorf("unexpected volume id passed to volume service: %d", id)
		}
		return existingVolume, nil
	}

	env.volumeMountService.UnpublishFunc = func(volume *csi.Volume, targetPath string) error {
		if volume != existingVolume {
			t.Errorf("unexpected volume passed to volume mount service: %v", volume)
		}
		if targetPath != "target" {
			t.Errorf("unexpected target path passed to volume service: %s", targetPath)
		}
		return nil
	}

	_, err := env.service.NodeUnpublishVolume(env.ctx, &proto.NodeUnpublishVolumeRequest{
		VolumeId:   "1",
		TargetPath: "target",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNodeServiceNodeUnpublishVolumeNotFound(t *testing.T) {
	env := newNodeServerTestEnv()

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		return nil, volumes.ErrVolumeNotFound
	}

	_, err := env.service.NodeUnpublishVolume(env.ctx, &proto.NodeUnpublishVolumeRequest{
		VolumeId:   "1",
		TargetPath: "target",
	})
	if grpc.Code(err) != codes.NotFound {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNodeServiceNodeUnpublishUnpublishError(t *testing.T) {
	env := newNodeServerTestEnv()

	env.volumeService.GetByIDFunc = func(ctx context.Context, id uint64) (*csi.Volume, error) {
		return &csi.Volume{}, nil
	}

	env.volumeMountService.UnpublishFunc = func(volume *csi.Volume, targetPath string) error {
		return io.EOF
	}

	_, err := env.service.NodeUnpublishVolume(env.ctx, &proto.NodeUnpublishVolumeRequest{
		VolumeId:   "1",
		TargetPath: "target",
	})
	if grpc.Code(err) != codes.Internal {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNodeServiceNodeUnpublishVolumeInputErrors(t *testing.T) {
	env := newNodeServerTestEnv()

	testCases := []struct {
		Name string
		Req  *proto.NodeUnpublishVolumeRequest
		Code codes.Code
	}{
		{
			Name: "empty volume id",
			Req: &proto.NodeUnpublishVolumeRequest{
				TargetPath: "target",
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "empty target path",
			Req: &proto.NodeUnpublishVolumeRequest{
				VolumeId: "1",
			},
			Code: codes.InvalidArgument,
		},
		{
			Name: "invalid volume id",
			Req: &proto.NodeUnpublishVolumeRequest{
				VolumeId:   "xxx",
				TargetPath: "target",
			},
			Code: codes.NotFound,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			_, err := env.service.NodeUnpublishVolume(env.ctx, testCase.Req)
			if grpc.Code(err) != testCase.Code {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}

func TestNodeServiceNodeGetCapabilities(t *testing.T) {
	env := newNodeServerTestEnv()

	req := &proto.NodeGetCapabilitiesRequest{}
	resp, err := env.service.NodeGetCapabilities(env.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if c := len(resp.Capabilities); c != 1 {
		t.Fatalf("unexpected number of capabilities: %d", c)
	}

	cap1rpc := resp.Capabilities[0].GetRpc()
	if cap1rpc == nil {
		t.Fatal("unexpected capability at index 0")
	}
	if cap1rpc.Type != proto.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME {
		t.Errorf("unexpected type: %s", cap1rpc.Type)
	}
}

func TestNodeServiceNodeGetInfo(t *testing.T) {
	env := newNodeServerTestEnv()

	req := &proto.NodeGetInfoRequest{}
	resp, err := env.service.NodeGetInfo(env.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.NodeId != "1" {
		t.Errorf("unexpected node id: %s", resp.NodeId)
	}
	if resp.MaxVolumesPerNode != MaxVolumesPerNode {
		t.Errorf("unexpected max volumes per node: %d", resp.MaxVolumesPerNode)
	}
}
