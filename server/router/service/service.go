package service

import (
	"github.com/hyperhq/hyperd/server/router"
	"github.com/hyperhq/hyperd/server/router/local"
)

// serverRouter is a router to talk with the server controller.
type serviceRouter struct {
	backend Backend
	routes  []router.Route
}

// NewRouter initializes a new systemRouter
func NewRouter(b Backend) router.Router {
	r := &serviceRouter{
		backend: b,
	}

	r.routes = []router.Route{
		// GET
		local.NewGetRoute("/service/list", r.getServices),
		// POST
		local.NewPostRoute("/service/add", r.postServiceAdd),
		local.NewPostRoute("/service/update", r.postServiceUpdate),
		// PUT
		// DELETE
		local.NewDeleteRoute("/service", r.deleteService),
	}

	return r
}

// Routes return all the API routes dedicated to the docker system.
func (s *serviceRouter) Routes() []router.Route {
	return s.routes
}
