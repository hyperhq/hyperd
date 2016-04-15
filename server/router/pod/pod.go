package pod

import (
	"github.com/hyperhq/hyperd/server/router"
	"github.com/hyperhq/hyperd/server/router/local"
)

// podRouter is a router to talk with the pod controller.
type podRouter struct {
	backend Backend
	routes  []router.Route
}

// NewRouter initializes a new systemRouter
func NewRouter(b Backend) router.Router {
	r := &podRouter{
		backend: b,
	}

	r.routes = []router.Route{
		// GET
		local.NewGetRoute("/pod/info", r.getPodInfo),
		local.NewGetRoute("/pod/stats", r.getPodStats),
		local.NewGetRoute("/list", r.getList),
		// POST
		local.NewPostRoute("/pod/create", r.postPodCreate),
		local.NewPostRoute("/pod/labels", r.postPodLabels),
		local.NewPostRoute("/pod/start", r.postPodStart),
		local.NewPostRoute("/pod/stop", r.postPodStop),
		local.NewPostRoute("/pod/kill", r.postPodKill),
		local.NewPostRoute("/pod/pause", r.postPodPause),
		local.NewPostRoute("/pod/unpause", r.postPodUnpause),
		local.NewPostRoute("/vm/create", r.postVmCreate),
		// PUT
		// DELETE
		local.NewDeleteRoute("/pod", r.deletePod),
		local.NewDeleteRoute("/vm", r.deleteVm),
	}

	return r
}

// Routes return all the API routes dedicated to the docker system.
func (s *podRouter) Routes() []router.Route {
	return s.routes
}
