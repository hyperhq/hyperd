package serverrpc

import (
	"bytes"
	"io"
	"io/ioutil"

	enginetypes "github.com/docker/engine-api/types"
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

// ImagePull pulls a image from registry
func (s *ServerRPC) ImagePull(req *types.ImagePullRequest, stream types.PublicAPI_ImagePullServer) error {
	glog.V(3).Infof("ImagePull with request %s", req.String())

	authConfig := &enginetypes.AuthConfig{}
	if req.Auth != nil {
		authConfig = &enginetypes.AuthConfig{
			Username:      req.Auth.Username,
			Password:      req.Auth.Password,
			Auth:          req.Auth.Auth,
			Email:         req.Auth.Email,
			ServerAddress: req.Auth.Serveraddress,
			RegistryToken: req.Auth.Registrytoken,
		}
	}

	r, w := io.Pipe()

	var pullResult error
	var complete = false

	go func() {
		defer r.Close()
		for {
			data := make([]byte, 512)
			n, err := r.Read(data)
			if err == io.EOF {
				if complete {
					break
				} else {
					continue
				}
			}

			if err != nil {
				glog.Errorf("Read image pull stream error: %v", err)
				return
			}

			if err := stream.Send(&types.ImagePullResponse{Data: data[:n]}); err != nil {
				glog.Errorf("Send image pull  progress to stream error: %v", err)
				return
			}
		}
	}()

	pullResult = s.daemon.CmdImagePull(req.Image, req.Tag, authConfig, nil, w)
	complete = true

	return pullResult
}

// ImageLoad loads an image from a tar archive or STDIN
func (s *ServerRPC) ImageLoad(stream types.PublicAPI_ImageLoadServer) error {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	var loadResult error
	go func() {
		defer outReader.Close()
		buf := make([]byte, 512)
		for {
			nr, err := outReader.Read(buf)
			if nr > 0 {
				if err := stream.Send(&types.ImageLoadResponse{buf[:nr]}); err != nil {
					glog.Errorf("Send to stream error: %v", err)
					return
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				glog.Errorf("Read from pipe error: %v", err)
				return
			}
		}

	}()

	go func() {
		defer inWriter.Close()
		for {
			req, err := stream.Recv()
			if err != nil && err != io.EOF {
				glog.Errorf("Receive from stream error: %v", err)
				return
			}
			if req != nil && req.Data != nil {
				nw, ew := inWriter.Write(req.Data)
				if ew != nil {
					glog.Errorf("Write pipe error: %v", ew)
					return
				}
				if nw != len(req.Data) {
					glog.Errorf("Write data length is not enougt, write: %d success: %d", len(req.Data), nw)
					return
				}
			}
			if err == io.EOF {
				break
			}
		}

	}()
	loadResult = s.daemon.CmdImageLoad(inReader, outWriter)
	return loadResult
}

// ImagePush pushes a local image to registry
func (s *ServerRPC) ImagePush(req *types.ImagePushRequest, stream types.PublicAPI_ImagePushServer) error {
	glog.V(3).Infof("ImagePush with request %s", req.String())

	authConfig := &enginetypes.AuthConfig{}
	if req.Auth != nil {
		authConfig = &enginetypes.AuthConfig{
			Username:      req.Auth.Username,
			Password:      req.Auth.Password,
			Auth:          req.Auth.Auth,
			Email:         req.Auth.Email,
			ServerAddress: req.Auth.Serveraddress,
			RegistryToken: req.Auth.Registrytoken,
		}
	}

	buffer := bytes.NewBuffer([]byte{})
	var pushResult error
	var complete = false
	go func() {
		pushResult = s.daemon.CmdImagePush(req.Repo, req.Tag, authConfig, nil, buffer)
		complete = true
	}()

	for {
		data, err := ioutil.ReadAll(buffer)
		if err == io.EOF {
			if complete {
				break
			} else {
				continue
			}
		}

		if err != nil {
			glog.Errorf("Read image push stream error: %v", err)
			return err
		}

		if err := stream.Send(&types.ImagePushResponse{Data: data}); err != nil {
			return err
		}
	}

	return pushResult
}

// ImageRemove deletes a image from hyperd
func (s *ServerRPC) ImageRemove(ctx context.Context, req *types.ImageRemoveRequest) (*types.ImageRemoveResponse, error) {
	glog.V(3).Infof("ImageDelete with request %s", req.String())

	resp, err := s.daemon.CmdImageDelete(req.Image, req.Force, req.Prune)
	if err != nil {
		glog.Errorf("DeleteImage failed: %v", err)
		return nil, err
	}

	return &types.ImageRemoveResponse{
		Images: resp,
	}, nil
}
