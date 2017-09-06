package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hyperhq/hyperd/types"
)

type PortMappingList struct {
	PortMappings []*types.PortMapping `json:"portMappings"`
}

func (c *Client) ListPortMappings(podId string) ([]*types.PortMapping, error) {
	path := fmt.Sprintf("/pod/%s/portmappings", podId)

	body, code, err := readBody(c.call("GET", path, nil, nil))
	if code == http.StatusNotFound {
		return nil, fmt.Errorf("pod %s not found", podId)
	} else if err != nil {
		return nil, err
	}

	var pms PortMappingList
	err = json.Unmarshal(body, &pms)
	if err != nil {
		return nil, err
	}

	return pms.PortMappings, nil
}

func (c *Client) AddPortMappings(podId string, pms []*types.PortMapping) error {
	path := fmt.Sprintf("/pod/%s/portmappings/add", podId)
	r, code, err := readBody(c.call("PUT", path, pms, nil))

	if code == http.StatusNoContent || code == http.StatusOK {
		return nil
	} else if code == http.StatusNotFound {
		return fmt.Errorf("pod %s not found", podId)
	} else if err != nil {
		return err
	} else {
		return fmt.Errorf("unexpect response code %d: %s", code, string(r))
	}
}

func (c *Client) DeletePortMappings(podId string, pms []*types.PortMapping) error {
	path := fmt.Sprintf("/pod/%s/portmappings/delete", podId)
	r, code, err := readBody(c.call("PUT", path, pms, nil))

	if code == http.StatusNoContent || code == http.StatusOK {
		return nil
	} else if code == http.StatusNotFound {
		return fmt.Errorf("pod %s not found", podId)
	} else if err != nil {
		return err
	} else {
		return fmt.Errorf("unexpect response code %d: %s", code, string(r))
	}
}
