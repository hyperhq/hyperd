package serverrpc

import (
	"github.com/golang/glog"
	"github.com/hyperhq/hyperd/types"
	"golang.org/x/net/context"
)

// ImageList implements GET /images/get
func (s *ServerRPC) ImageList(ctx context.Context, req *types.ImageListRequest) (*types.ImageListResponse, error) {
	glog.V(3).Infof("ImageList with request %s", req.String())

	images, err := s.daemon.Daemon.Images(req.FilterArgs, req.Filter, req.All)
	if err != nil {
		glog.Errorf("ImageList error: %v", err)
		return nil, err
	}

	result := make([]*types.ImageInfo, 0, len(images))
	for _, image := range images {
		result = append(result, &types.ImageInfo{
			Id:          image.ID,
			ParentID:    image.ParentID,
			RepoTags:    image.RepoTags,
			RepoDigests: image.RepoDigests,
			Created:     image.Created,
			VirtualSize: image.VirtualSize,
			Labels:      image.Labels,
		})
	}

	return &types.ImageListResponse{
		ImageList: result,
	}, nil
}
