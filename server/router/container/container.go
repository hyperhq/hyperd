package container

import (
	"github.com/hyperhq/hyperd/server/router"
	"github.com/hyperhq/hyperd/server/router/local"
)

// containerRouter is a router to talk with the container controller
type containerRouter struct {
	backend Backend
	routes  []router.Route
}

// NewRouter initializes a new container router
func NewRouter(b Backend) router.Router {
	r := &containerRouter{
		backend: b,
	}
	r.initRoutes()
	return r
}

// Routes returns the available routers to the container controller
func (r *containerRouter) Routes() []router.Route {
	return r.routes
}

// initRoutes initializes the routes in container router
func (r *containerRouter) initRoutes() {
	r.routes = []router.Route{
		// GET
		local.NewGetRoute("/container/info", r.getContainerInfo),
		local.NewGetRoute("/container/logs", r.getContainerLogs),
		local.NewGetRoute("/exitcode", r.getExitCode),
		// POST
		local.NewPostRoute("/container/create", r.postContainerCreate),
		local.NewPostRoute("/container/rename", r.postContainerRename),
		local.NewPostRoute("/container/commit", r.postContainerCommit),
		local.NewPostRoute("/container/stop", r.postContainerStop),
		local.NewPostRoute("/container/kill", r.postContainerKill),
		local.NewPostRoute("/exec/create", r.postContainerExecCreate),
		local.NewPostRoute("/exec/start", r.postContainerExecStart),
		local.NewPostRoute("/attach", r.postContainerAttach),
		local.NewPostRoute("/tty/resize", r.postTtyResize),
		// PUT
		// DELETE
	}
}
