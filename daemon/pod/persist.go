package pod

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/hyperd/daemon/daemondb"
	"github.com/hyperhq/hyperd/types"
)

/// Layout of Persistent Info of a Pod:
/// PL-{Pod.Id()}: overall layout of the persistent Info of the Pod
///         |`- id/name
///         |`- container id list
///         |`- volume name list
///          `- interface id list
/// SB-{Pod.Id()}: sandbox persistent Info, retrieved from runV
/// PS-{Pod.Id()}: Global Part of Pod Spec
/// PM-{Pod.Id()}: Pod level metadata that could be changed
///         |`- services: service list
///          `- labels
/// CX-{Container.Id()} Container Persistent Info
/// VX-{Pod.ID()}-{Volume.Name()} Volume Persist Info
/// IF-{Pod.ID()}-{Inf.Id()}

const (
	LAYOUT_KEY_PREFIX = "PL-"
	LAYOUT_KEY_FMT    = "PL-%s"
	SB_KEY_FMT        = "SB-%s"
	PS_KEY_FMT        = "PS-%s"
	PM_KEY_FMT        = "PM-%s"
	CX_KEY_FMT        = "CX-%s"
	VX_KEY_FMT        = "VX-%s-%s"
	IF_KEY_FMT        = "IF-%s-%s"
)

func LoadAllPods(db *daemondb.DaemonDB) chan *types.PersistPodLayout {
	kvchan := db.PrefixList2Chan([]byte(LAYOUT_KEY_PREFIX), nil)
	if kvchan == nil {
		return nil
	}
	ch := make(chan *types.PersistPodLayout, 128)
	go func() {
		for {
			kv, ok := <-kvchan
			if !ok {
				hlog.Log(INFO, "layout loading finished")
				close(ch)
				return
			}
			hlog.Log(TRACE, "loading layout of container %s", string(kv.K))

			var layout types.PersistPodLayout
			err := proto.Unmarshal(kv.V, &layout)
			if err != nil {
				hlog.Log(ERROR, "failed to decode layout of contaienr %s: %v", string(kv.K), err)
				continue
			}
			ch <- &layout
		}
	}()
	return ch
}

func LoadXPod(factory *PodFactory, layout *types.PersistPodLayout) (*XPod, error) {
	spec, err := loadGloabalSpec(factory.db, layout.Id)
	if err != nil {
		return nil, err
	}

	p, err := newXPod(factory, spec)
	if err != nil {
		hlog.Log(ERROR, "failed to create pod from spec: %v", err)
		//remove spec from daemonDB
		//remove vm from daemonDB
		return nil, err
	}
	err = p.reserveNames(spec.Containers)
	if err != nil {
		return nil, err
	}

	for _, ix := range layout.Interfaces {
		if err := p.loadInterface(ix); err != nil {
			return nil, err
		}
	}

	for _, vid := range layout.Volumes {
		if err := p.loadVolume(vid); err != nil {
			return nil, err
		}
	}

	for _, cid := range layout.Containers {
		if err := p.loadContainer(cid); err != nil {
			return nil, err
		}
	}

	err = p.loadSandbox()
	if err != nil {
		//remove vm from daemonDB
		return nil, err
	}

	err = p.loadPodMeta()
	if err != nil {
		return nil, err
	}

	//resume logging
	if p.status == S_POD_RUNNING {
		for _, c := range p.containers {
			c.startLogging()
		}
	}

	// don't need to reserve name again, because this is load
	return p, nil
}

func (p *XPod) savePod() error {
	var (
		containers = make([]string, 0, len(p.containers))
		volumes    = make([]string, 0, len(p.volumes))
		interfaces = make([]string, 0, len(p.interfaces))
	)

	if err := p.saveGlobalSpec(); err != nil {
		return err
	}

	if err := p.savePodMeta(); err != nil {
		return err
	}

	for cid, c := range p.containers {
		containers = append(containers, cid)
		if err := c.saveContainer(); err != nil {
			return err
		}
	}

	for vid, v := range p.volumes {
		volumes = append(volumes, vid)
		if err := v.saveVolume(); err != nil {
			return err
		}
	}

	for inf, i := range p.interfaces {
		interfaces = append(interfaces, inf)
		if err := i.saveInterface(); err != nil {
			return err
		}
	}

	pl := &types.PersistPodLayout{
		Id:         p.Id(),
		Containers: containers,
		Volumes:    volumes,
		Interfaces: interfaces,
	}
	return saveMessage(p.factory.db, fmt.Sprintf(LAYOUT_KEY_FMT, p.Id()), pl, p, "pod layout")
}

func (p *XPod) saveGlobalSpec() error {
	return saveMessage(p.factory.db, fmt.Sprintf(PS_KEY_FMT, p.Id()), p.globalSpec, p, "global spec")
}

func loadGloabalSpec(db *daemondb.DaemonDB, id string) (*types.UserPod, error) {
	var spec types.UserPod
	err := loadMessage(db, fmt.Sprintf(LAYOUT_KEY_FMT, id), &spec, nil, fmt.Sprintf("spec for %s", id))
	if err != nil {
		return nil, err
	}
	return &spec, nil
}

func (p *XPod) savePodMeta() error {
	meta := &types.PersistPodMeta{
		Id:       p.Id(),
		Services: p.services,
		Labels:   p.labels,
	}
	if p.info != nil {
		meta.CreatedAt = p.info.CreatedAt
	}
	return saveMessage(p.factory.db, fmt.Sprintf(PM_KEY_FMT, p.Id()), meta, p, "pod meta")
}

func (p *XPod) loadPodMeta() error {
	var meta types.PersistPodMeta
	err := loadMessage(p.factory.db, fmt.Sprintf(PM_KEY_FMT, p.Id()), &meta, p, "pod meta")
	if err != nil {
		return err
	}
	p.initPodInfo()
	if meta.CreatedAt > 0 {
		p.info.CreatedAt = meta.CreatedAt
	}
	p.labels = meta.Labels
	p.services = meta.Services
	return nil
}

func (c *Container) saveContainer() error {
	cx := &types.PersistContainer{
		Id:       c.Id(),
		Pod:      c.p.Id(),
		Spec:     c.spec,
		Descript: c.descript,
	}
	return saveMessage(c.p.factory.db, fmt.Sprintf(CX_KEY_FMT, c.Id()), cx, c, "container info")
}

func (p *XPod) loadContainer(id string) error {
	var cx types.PersistContainer
	err := loadMessage(p.factory.db, fmt.Sprintf(CX_KEY_FMT, id), &cx, p, fmt.Sprintf("container info of %s", id))
	if err != nil {
		return err
	}
	c, err := newContainer(p, cx.Spec, false)
	if err != nil {
		p.Log(ERROR, "failed to reload container %s from spec: %v", id, err)
		return err
	}
	err = p.factory.registry.ReserveContainer(c.Id(), c.SpecName(), p.Id())
	if err != nil {
		p.Log(ERROR, "failed to register name of container %s (%s) during load", c.Id(), c.SpecName(), err)
		return err
	}
	p.containers[c.Id()] = c
	return nil
}

func (v *Volume) saveVolume() error {
	vx := &types.PersistVolume{
		Name:     v.spec.Name,
		Pod:      v.p.Id(),
		Spec:     v.spec,
		Descript: v.descript,
	}
	return saveMessage(v.p.factory.db, fmt.Sprintf(VX_KEY_FMT, v.p.Id(), v.spec.Name), vx, v, "volume info")
}

func (p *XPod) loadVolume(id string) error {
	var vx types.PersistVolume
	err := loadMessage(p.factory.db, fmt.Sprintf(VX_KEY_FMT, p.Id(), id), &vx, p, fmt.Sprintf("volume info of %s", id))
	if err != nil {
		return err
	}
	v := newVolume(p, vx.Spec)
	v.descript = vx.Descript
	v.status = S_VOLUME_CREATED
	p.volumes[v.spec.Name] = v
	return nil
}

func (inf *Interface) saveInterface() error {
	ix := &types.PersistInterface{
		Id:       inf.descript.Id,
		Pod:      inf.p.Id(),
		Spec:     inf.spec,
		Descript: inf.descript,
	}
	return saveMessage(inf.p.factory.db, fmt.Sprintf(IF_KEY_FMT, inf.p.Id(), inf.descript.Id), ix, inf, "interface info")
}

func (p *XPod) loadInterface(id string) error {
	var ix types.PersistInterface
	err := loadMessage(p.factory.db, fmt.Sprintf(IF_KEY_FMT, p.Id(), id), &ix, p, fmt.Sprintf("inf info of %s", id))
	if err != nil {
		return err
	}
	inf := newInterface(p, ix.Spec)
	inf.descript = ix.Descript
	p.interfaces[inf.descript.Id] = inf
	return nil
}

func (p *XPod) loadSandbox() error {
	var sb types.SandboxPersistInfo
	err := loadMessage(p.factory.db, fmt.Sprintf(SB_KEY_FMT, p.Id()), &sb, p, "load sandbox info")
	if err != nil {
		return err
	}
	return p.reconnectSandbox(sb.Id, sb.PersistInfo)
}

func saveMessage(db *daemondb.DaemonDB, key string, message proto.Message, owner hlog.LogOwner, op string) error {
	pm, err := proto.Marshal(message)
	if err != nil {
		hlog.HLog(ERROR, owner, 2, "failed to serialize %s: %v", op, err)
		return err
	}
	err = db.Update([]byte(key), pm)
	if err != nil {
		hlog.HLog(ERROR, owner, 2, "failed to write %s to db: %v", op, err)
		return err
	}
	hlog.HLog(DEBUG, owner, 2, "%s serialized to db", op)
	return nil
}

func loadMessage(db *daemondb.DaemonDB, key string, message proto.Message, owner hlog.LogOwner, op string) error {
	v, err := db.Get([]byte(key))
	if err != nil {
		hlog.HLog(ERROR, owner, 2, "failed to load %s: %v", op, err)
		return err
	}
	err = proto.Unmarshal(v, message)
	if err != nil {
		hlog.HLog(ERROR, owner, 2, "failed to unpack loaded %s: %v", op, err)
		return err
	}
	return nil
}
