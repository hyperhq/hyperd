package pod

import (
	"github.com/docker/docker/pkg/stringid"
	apitypes "github.com/hyperhq/hyperd/types"
	"strings"
)

type ContainerBuffer struct {
	Id   string
	P    *XPod
	Spec *apitypes.UserContainer
}

func (cb *ContainerBuffer) info() *apitypes.Container {
	cinfo := &apitypes.Container{
		Name:            "/" + cb.Spec.Name,
		ContainerID:     cb.Id,
		Image:           cb.Spec.Image,
		Commands:        cb.Spec.Command,
		WorkingDir:      cb.Spec.Workdir,
		Labels:          cb.Spec.Labels,
		Ports:           make([]*apitypes.ContainerPort, 0, len(cb.Spec.Ports)),
		VolumeMounts:    make([]*apitypes.VolumeMount, 0, len(cb.Spec.Volumes)),
		Env:             make([]*apitypes.EnvironmentVar, 0, len(cb.Spec.Envs)),
		Tty:             cb.Spec.Tty,
		ImagePullPolicy: "",
	}
	for _, port := range cb.Spec.Ports {
		cinfo.Ports = append(cinfo.Ports, &apitypes.ContainerPort{
			HostPort:      port.HostPort,
			ContainerPort: port.ContainerPort,
			Protocol:      port.Protocol,
		})
	}
	for _, vol := range cb.Spec.Volumes {
		cinfo.VolumeMounts = append(cinfo.VolumeMounts, &apitypes.VolumeMount{
			Name:      vol.Volume,
			MountPath: vol.Path,
			ReadOnly:  vol.ReadOnly,
		})
	}
	for _, env := range cb.Spec.Envs {
		cinfo.Env = append(cinfo.Env, &apitypes.EnvironmentVar{
			Env:   env.Env,
			Value: env.Value,
		})
	}
	return cinfo
}

func (cb *ContainerBuffer) infoStatus() *apitypes.ContainerStatus {
	s := &apitypes.ContainerStatus{
		Name:        cb.Spec.Name,
		ContainerID: cb.Id,
		Waiting:     &apitypes.WaitingStatus{Reason: "Pending"},
		Running:     &apitypes.RunningStatus{StartedAt: ""},
		Terminated:  &apitypes.TermStatus{},
		Phase:       "pending",
	}
	return s
}

func (p *XPod) AddContainerBuffer(c *apitypes.UserContainer) (*ContainerBuffer, error) {
	cid := stringid.GenerateNonCryptoID()

	cb := &ContainerBuffer{
		Id:   cid,
		P:    p,
		Spec: c,
	}
	p.statusLock.Lock()
	p.containerBuffers[cid] = cb
	p.statusLock.Unlock()

	err := p.factory.registry.ReserveContainer(cid, c.Name, p.Id())
	if err != nil {
		p.RemoveContainerBuffer(cb)
		return nil, err
	}

	return cb, nil
}

func (p *XPod) RemoveContainerBuffer(cb *ContainerBuffer) {
	p.statusLock.Lock()
	defer p.statusLock.Unlock()
	delete(p.containerBuffers, cb.Id)
}

func (p *XPod) RemoveContainerBufferAll(cb *ContainerBuffer) {
	p.factory.registry.ReleaseContainer(cb.Id, cb.Spec.Name)

	p.RemoveContainerBuffer(cb)
}

func (p *XPod) AppendContainerBufferStatus(result []*apitypes.ContainerListResult) []*apitypes.ContainerListResult {
	p.statusLock.RLock()
	defer p.statusLock.RUnlock()
	for id, cb := range p.containerBuffers {
		result = append(result, &apitypes.ContainerListResult{
			ContainerID:   id,
			ContainerName: cb.Spec.Name,
			PodID:         p.Id(),
			Status:        "pending",
		})
	}
	return result
}

func (p *XPod) AppendContainerBufferStatusString(result []string) []string {
	p.statusLock.RLock()
	defer p.statusLock.RUnlock()
	for id, cb := range p.containerBuffers {
		result = append(result, strings.Join([]string{id, cb.Spec.Name, p.Id(), "pending"}, ":"))
	}
	return result
}

func (p *XPod) ContainerBufferInfo(cid string) *apitypes.ContainerInfo {
	p.statusLock.RLock()
	defer p.statusLock.RUnlock()
	if cb, ok := p.containerBuffers[cid]; ok {
		//Not set CreatedAt
		ci := &apitypes.ContainerInfo{
			PodID:     p.Id(),
			Container: cb.info(),
			Status:    cb.infoStatus(),
		}
		return ci
	}
	return nil
}
