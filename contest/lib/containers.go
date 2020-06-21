package contestlib

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	dockerNetwork "github.com/docker/docker/api/types/network"
	dockerClient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/spikeekips/mitum/base/key"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/storage"
	"github.com/spikeekips/mitum/util"
	"github.com/spikeekips/mitum/util/encoder"
	jsonenc "github.com/spikeekips/mitum/util/encoder/json"
	"github.com/spikeekips/mitum/util/logging"
)

var (
	MongodbImage         = "mongo:latest"
	MongodbContainerName = "contest-mongodb"
	MongodbExternalPort  = "37017"
	ContainerLabel       = "contest"
)

type Containers struct {
	sync.RWMutex
	*logging.Logging
	encs            *encoder.Encoders
	client          *dockerClient.Client
	image           string
	runner          string
	networkName     string
	dockerNetworkID string
	design          *ContestDesign
	outputDir       string
	containers      map[string]*Container
	mongodbIP       string
	exitAfter       time.Duration
	eventChan       chan *Event
	genesisNode     *Container
	ports           map[string]int
	pks             map[string]key.Privatekey
}

func NewContainers(
	dc *dockerClient.Client,
	encs *encoder.Encoders,
	image,
	runner,
	networkName,
	dockerNetworkID string,
	design *ContestDesign,
	outputDir string,
	exitAfter time.Duration,
) (*Containers, error) {
	return &Containers{
		Logging: logging.NewLogging(func(c logging.Context) logging.Emitter {
			return c.Str("module", "contest-containers")
		}),
		client:          dc,
		encs:            encs,
		image:           image,
		runner:          runner,
		networkName:     networkName,
		dockerNetworkID: dockerNetworkID,
		design:          design,
		outputDir:       outputDir,
		exitAfter:       exitAfter,
		ports:           map[string]int{},
		pks:             map[string]key.Privatekey{},
	}, nil
}

func (cts *Containers) Kill(sig string) error {
	cts.Log().Debug().Msg("trying to kill")

	if err := KillContainers(cts.client, sig); err != nil {
		return err
	}
	cts.Log().Debug().Msg("containers killed")

	return nil
}

func (cts *Containers) Stop() error {
	cts.Log().Debug().Msg("trying to stop")

	if err := StopContainers(cts.client); err != nil {
		return err
	}

	cts.Log().Debug().Msg("containers stoped")

	return nil
}

func (cts *Containers) Clean() error {
	cts.Log().Debug().Msg("trying to clean")

	if err := StopContainers(cts.client); err != nil {
		return err
	}
	cts.Log().Debug().Msg("containers stoped")

	if err := CleanContainers(cts.client, cts.Log()); err != nil {
		return err
	}
	cts.Log().Debug().Msg("containers cleaned")

	if err := ContainersPrune(cts.client); err != nil {
		return err
	}
	cts.Log().Debug().Msg("containers pruned")

	if err := VolumesPrune(cts.client); err != nil {
		return err
	}
	cts.Log().Debug().Msg("volumes pruned")

	return nil
}

func (cts *Containers) createContainers() ([]*Container, error) {
	cts.Log().Debug().Str("outputDir", cts.outputDir).Msg("trying to create containers")

	cs := make([]*Container, len(cts.design.Nodes))
	for i, d := range cts.design.Nodes {
		if c, err := cts.createContainer(d); err != nil {
			return nil, xerrors.Errorf("failed to create container: %w", err)
		} else {
			cs[i] = c
		}
	}

	rds := make([]*RemoteDesign, len(cs))
	for i, c := range cs {
		rds[i] = c.RemoteDesign()
	}

	for i, c := range cs {
		c.rds = rds

		if b, err := yaml.Marshal(c.NodeDesign(i == 0)); err != nil {
			return nil, xerrors.Errorf("failed to create node design: %w", err)
		} else {
			designFile := filepath.Join(cts.outputDir, fmt.Sprintf("design-%s.yml", c.name))
			if err := ioutil.WriteFile(designFile, b, 0o600); err != nil {
				return nil, err
			}
			c.designFile = designFile
		}
	}

	return cs, nil
}

func (cts *Containers) createContainer(d *ContestNodeDesign) (*Container, error) {
	var errWriter io.Writer
	errWriterFile := filepath.Join(cts.outputDir, fmt.Sprintf("error-%s.log", d.Address()))
	if f, err := os.Create(errWriterFile); err != nil {
		return nil, xerrors.Errorf("failed to create error log file: %w", err)
	} else {
		errWriter = f
	}

	container := &Container{
		encs:            cts.encs,
		contestDesign:   cts.design,
		design:          d,
		name:            d.Address(),
		image:           cts.image,
		client:          cts.client,
		port:            cts.port(d.Address()),
		runner:          cts.runner,
		privatekey:      cts.pk(d.Address()),
		networkName:     cts.networkName,
		dockerNetworkID: cts.dockerNetworkID,
		exitAfter:       cts.exitAfter,
		eventChan:       cts.eventChan,
		shareDir:        cts.outputDir,
		errWriter:       errWriter,
	}

	address := d.Address()
	container.Logging = logging.NewLogging(func(c logging.Context) logging.Emitter {
		return c.Str("module", "contest-container").Str("address", address)
	})
	_ = container.SetLogger(cts.Log())

	return container, nil
}

func (cts *Containers) Create() error {
	var cs []*Container
	if c, err := cts.createContainers(); err != nil {
		return err
	} else {
		cs = c
	}

	mcs := map[string]*Container{}
	for i := range cs {
		mcs[cs[i].name] = cs[i]
	}

	if c, err := cts.createContainer(cs[0].design); err != nil {
		return xerrors.Errorf("failed to create genesis node: %w", err)
	} else {
		cts.genesisNode = c
		cts.genesisNode.rds = cs[0].rds
		cts.genesisNode.designFile = cs[0].designFile
	}

	cts.containers = mcs

	return nil
}

func (cts *Containers) Ready() error {
	if err := cts.Create(); err != nil {
		return xerrors.Errorf("failed to create containers: %w", err)
	}

	if err := cts.RunStorage(); err != nil {
		return xerrors.Errorf("failed to run storage: %w", err)
	}

	if err := cts.initializeByGenesisNode(); err != nil {
		return xerrors.Errorf("failed to initilaize by genesis node: %w", err)
	}

	cts.eventChan <- EmptyEvent().
		Add("module", "contest-containers").
		Add("m", "ready")

	return nil
}

func (cts *Containers) Run() error {
	return nil
}

func (cts *Containers) RunNodes(nodes []string) error {
	l := cts.Log().WithLogger(func(ctx logging.Context) logging.Emitter {
		return ctx.Strs("nodes", nodes)
	})
	l.Debug().Msg("trying to run containers")

	var wg sync.WaitGroup
	wg.Add(len(nodes))

	for _, name := range nodes {
		var container *Container
		if c, found := cts.containers[name]; !found {
			return xerrors.Errorf("container name, %q not found", name)
		} else {
			container = c
		}

		if running, err := container.IsRunning(); err != nil {
			return err
		} else if running {
			l.Debug().Msg("node already running")

			continue
		}

		go func(c *Container) {
			defer wg.Done()

			if err := c.run(false); err != nil {
				cts.Log().Error().Err(err).Str("address", c.name).Msg("failed to run")
			}

			cts.eventChan <- EmptyEvent().
				Add("module", "contest-containers").
				Add("node", c.name).
				Add("m", "node started")
		}(container)
	}

	wg.Wait()

	return nil
}

func (cts *Containers) initializeByGenesisNode() error {
	cts.Log().Debug().Msg("trying to initialize")

	if err := cts.genesisNode.run(true); err != nil {
		return err
	} else if err := CleanContainer(cts.client, cts.genesisNode.id, cts.Log()); err != nil {
		return err
	}

	var gstorage storage.Storage
	if st, err := cts.genesisNode.Storage(); err != nil {
		return err
	} else {
		gstorage = st
	}

	cts.Log().Debug().Msg("sync storage")
	for _, c := range cts.containers {
		if c.name == cts.genesisNode.name {
			continue
		}

		cts.Log().Debug().Str("from", cts.genesisNode.name).Str("to", c.name).Msg("trying to sync storage")
		if st, err := c.Storage(); err != nil {
			return err
		} else if err := st.Copy(gstorage); err != nil {
			return err
		}

		cts.Log().Debug().Str("from", cts.genesisNode.name).Str("to", c.name).Msg("storage synced")
	}

	cts.Log().Debug().Msg("initialized")

	return nil
}

func (cts *Containers) RunStorage() error {
	cts.Log().Debug().Msg("trying to run mongodb")

	var id string
	if r, err := cts.client.ContainerCreate(context.Background(),
		&container.Config{
			Image:        MongodbImage,
			Labels:       map[string]string{ContainerLabel: "mongodb"},
			ExposedPorts: nat.PortSet{"27017/tcp": struct{}{}},
		},
		&container.HostConfig{
			PortBindings: nat.PortMap{
				"27017/tcp": []nat.PortBinding{
					{HostIP: "", HostPort: MongodbExternalPort},
				},
			},
		},
		&dockerNetwork.NetworkingConfig{
			EndpointsConfig: map[string]*dockerNetwork.EndpointSettings{
				cts.networkName: {NetworkID: cts.dockerNetworkID},
			},
		},
		MongodbContainerName,
	); err != nil {
		return xerrors.Errorf("failed to create mongodb container: %w", err)
	} else {
		id = r.ID
	}

	if err := cts.client.ContainerStart(context.Background(), id, types.ContainerStartOptions{}); err != nil {
		return xerrors.Errorf("failed to run mongodb container: %w", err)
	}

	if r, err := ContainerInspect(cts.client, id); err != nil {
		return err
	} else {
		cts.mongodbIP = r.NetworkSettings.IPAddress

		for _, c := range cts.containers {
			c.mongodbIP = cts.mongodbIP
		}
	}

	if err := ContainerWaitCheck(cts.client, id, "waiting for connections on port", 100); err != nil {
		return xerrors.Errorf("failed to run mongodb container: %w", err)
	}

	cts.Log().Debug().Msg("mongodb launched")

	return nil
}

func (cts *Containers) SetEventChan(ch chan *Event) {
	cts.eventChan = ch
}

func (cts *Containers) StorageURI(db string) string {
	return fmt.Sprintf("mongodb://%s:%s/%s", cts.mongodbIP, MongodbExternalPort, db)
}

func (cts *Containers) pk(name string) key.Privatekey {
	cts.RLock()
	if pk, found := cts.pks[name]; found {
		defer cts.RUnlock()

		return pk
	}
	cts.RUnlock()

	cts.Lock()
	defer cts.Unlock()

	pk, _ := key.NewBTCPrivatekey()
	cts.pks[name] = pk

	return pk
}

func (cts *Containers) port(name string) int {
	cts.RLock()
	if p, found := cts.ports[name]; found {
		defer cts.RUnlock()

		return p
	}
	cts.RUnlock()

	cts.Lock()
	defer cts.Unlock()

	for {
		p, err := util.FreePort("udp")
		if err != nil {
			<-time.After(time.Millisecond * 300)
			continue
		}

		var found bool
		for _, e := range cts.ports {
			if p == e {
				found = true

				break
			}
		}

		if !found {
			cts.ports[name] = p

			break
		}
	}

	return cts.ports[name]
}

func (cts *Containers) Runnings() ([]string, error) {
	var rs []string
	for _, c := range cts.containers {
		if running, err := c.IsRunning(); err != nil {
			return nil, err
		} else if running {
			rs = append(rs, c.name)
		}
	}

	return rs, nil
}

func (cts *Containers) GenesisNode() *Container {
	return cts.genesisNode
}

func (cts *Containers) Container(name string) (*Container, bool) {
	c, found := cts.containers[name]

	return c, found
}

type Container struct {
	encs *encoder.Encoders
	sync.RWMutex
	*logging.Logging
	contestDesign   *ContestDesign
	design          *ContestNodeDesign
	nodeDesign      NodeDesign
	name            string
	image           string
	client          *dockerClient.Client
	port            int
	runner          string
	privatekey      key.Privatekey
	id              string
	networkName     string
	dockerNetworkID string
	rds             []*RemoteDesign
	designFile      string
	errWriter       io.Writer
	mongodbIP       string
	shareDir        string
	exitAfter       time.Duration
	eventChan       chan *Event
}

func (ct *Container) ID() string {
	ct.RLock()
	defer ct.RUnlock()

	return ct.id
}

func (ct *Container) setID(id string) {
	ct.Lock()
	defer ct.Unlock()

	ct.id = id
}

func (ct *Container) IsRunning() (bool, error) {
	ct.RLock()
	defer ct.RUnlock()

	if len(ct.ID()) < 1 {
		return false, nil
	}

	return ContainerIsRunning(ct.client, ct.ID())
}

func (ct *Container) Name() string {
	return fmt.Sprintf("contest-node-%s", ct.name)
}

func (ct *Container) create() error {
	r, err := ct.client.ContainerCreate(
		context.Background(),
		ct.configCreate([]string{
			"/runner",
			"--log", "/dev/stdout",
			"--log", fmt.Sprintf("/share/base-%s.log", ct.name),
			"run",
			"--mem-prof", fmt.Sprintf("/share/%s-mem.prof", ct.name),
			"--cpu-prof", fmt.Sprintf("/share/%s-cpu.prof", ct.name),
			"--trace-prof", fmt.Sprintf("/share/%s-trace.prof", ct.name),
			"--exit-after", ct.exitAfter.String(),
			fmt.Sprintf("/share/design-%s.yml", ct.name),
		}),
		ct.configHost(),
		ct.configNetworking(),
		ct.Name(),
	)
	if err != nil {
		return xerrors.Errorf("failed to create container: %w", err)
	}

	ct.setID(r.ID)

	return nil
}

func (ct *Container) createInitialize() error {
	r, err := ct.client.ContainerCreate(
		context.Background(),
		ct.configCreate([]string{
			"/runner",
			"init",
			fmt.Sprintf("/share/design-%s.yml", ct.name),
		}),
		ct.configHost(),
		ct.configNetworking(),
		ct.Name(),
	)
	if err != nil {
		return xerrors.Errorf("failed to create container: %w", err)
	}

	ct.setID(r.ID)

	return nil
}

func (ct *Container) run(initialize bool) error {
	l := ct.Log().WithLogger(func(ctx logging.Context) logging.Emitter {
		return ctx.Bool("initialize", initialize)
	})
	l.Debug().Msg("trying to run")

	var createFunc func() error
	if initialize {
		createFunc = ct.createInitialize
	} else {
		createFunc = ct.create
	}

	if err := createFunc(); err != nil {
		return err
	}
	l.Debug().Msg("container created")

	if err := ct.client.ContainerStart(context.Background(), ct.ID(), types.ContainerStartOptions{}); err != nil {
		return xerrors.Errorf("failed to run container: %w", err)
	}
	l.Debug().Msg("container started")

	if cancel, err := ct.containerErr(); err != nil {
		return err
	} else {
		defer cancel()
	}

	if initialize {
		return ct.wait()
	} else {
		go func() {
			if err := ct.wait(); err != nil {
				ct.Log().Error().Err(err).Msg("failed to run container")
			}
		}()

		return nil
	}
}

func (ct *Container) wait() error {
	if cancel, err := ct.containerLog(); err != nil {
		return err
	} else {
		defer cancel()
	}

	if status, err := ContainerWait(ct.client, ct.ID()); err != nil {
		return err
	} else if status != 0 {
		return xerrors.Errorf("container exited abnormally: statuscode=%d", status)
	}

	ct.Log().Debug().Msg("ended")

	return nil
}

func (ct *Container) containerLog() (func(), error) {
	if ct.eventChan == nil {
		return func() {}, nil
	}

	logChan := make(chan []byte)

	// stdout: event log
	var cancel func()
	if c, err := ContainerLogs(ct.client, ct.ID(), logChan, true, false); err != nil {
		return nil, err
	} else {
		cancel = func() {
			c()
		}
	}

	go func() {
		for b := range logChan {
			if e, err := NewEvent(b); err != nil {
				ct.Log().Error().Err(err).Str("raw", string(b)).Msg("failed to parse event log")
			} else {
				ct.eventChan <- e.Add("_node", ct.name)
			}
		}
	}()

	return cancel, nil
}

func (ct *Container) containerErr() (func(), error) {
	errChan := make(chan []byte)
	var cancel func()
	if c, err := ContainerLogs(ct.client, ct.ID(), errChan, false, true); err != nil {
		return nil, err
	} else {
		cancel = func() {
			c()
		}
	}

	var errWriter io.Writer = ct.errWriter
	if ct.errWriter == nil {
		errWriter = ioutil.Discard
	}

	go func() {
		for b := range errChan {
			ct.Log().Error().Str("msg", string(b)).Msg("container error")
			if _, err := fmt.Fprintln(errWriter, string(b)); err != nil {
				ct.Log().Error().Err(err).Msg("failed to write error log")
			}
		}
	}()

	return cancel, nil
}

func (ct *Container) networkDesign() *NetworkDesign {
	return &NetworkDesign{
		Bind:    "0.0.0.0:54321",
		Publish: fmt.Sprintf("quic://%s:54321", ct.Name()),
	}
}

func (ct *Container) RemoteDesign() *RemoteDesign {
	return &RemoteDesign{
		Address:         ct.name,
		PublickeyString: jsonenc.ToString(ct.privatekey.Publickey()),
		Network:         ct.networkDesign().Publish,
	}
}

func (ct *Container) NodeDesign(isGenesisNode bool) NodeDesign {
	if ct.nodeDesign.Network != nil {
		return ct.nodeDesign
	}

	var nodes []*RemoteDesign // nolint
	for _, d := range ct.rds {
		if d.Address == ct.name {
			continue
		}

		nodes = append(nodes, d)
	}

	nd := NodeDesign{
		encs:             ct.encs,
		Address:          ct.name,
		PrivatekeyString: jsonenc.ToString(ct.privatekey),
		Storage:          ct.storageURIInternal(),
		NetworkIDString:  ct.contestDesign.Config.NetworkIDString,
		Network:          ct.networkDesign(),
		Nodes:            nodes,
		Component:        ct.design.Component,
	}

	if isGenesisNode {
		nd.GenesisPolicy = ct.contestDesign.Config.GenesisPolicy
	}

	if err := nd.IsValid(nil); err != nil {
		panic(err)
	}

	ct.nodeDesign = nd

	return nd
}

func (ct *Container) configCreate(cmd []string) *container.Config {
	return &container.Config{
		Cmd:        cmd,
		WorkingDir: "/",
		Tty:        false,
		Image:      ct.image,
		Labels: map[string]string{
			ContainerLabel: ct.name,
		},
		ExposedPorts: nat.PortSet{
			"54321/udp": struct{}{},
		},
	}
}

func (ct *Container) configHost() *container.HostConfig {
	return &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   filepath.Join(ct.shareDir, "runner"),
				Target:   "/runner",
				ReadOnly: true,
			},
			{
				Type:   mount.TypeBind,
				Source: ct.shareDir,
				Target: "/share",
			},
		},
		PortBindings: nat.PortMap{
			"54321/udp": []nat.PortBinding{
				{
					HostIP:   "",
					HostPort: fmt.Sprintf("%d", ct.port),
				},
			},
		},
	}
}

func (ct *Container) configNetworking() *dockerNetwork.NetworkingConfig {
	return &dockerNetwork.NetworkingConfig{
		EndpointsConfig: map[string]*dockerNetwork.EndpointSettings{
			ct.networkName: {
				NetworkID: ct.dockerNetworkID,
			},
		},
	}
}

func (ct *Container) storageURIInternal() string {
	return fmt.Sprintf("mongodb://%s:27017/contest_%s", MongodbContainerName, ct.name)
}

func (ct *Container) storageURIExternal() string {
	return fmt.Sprintf("mongodb://%s:%s/contest_%s", ct.mongodbIP, MongodbExternalPort, ct.name)
}

func (ct *Container) Storage() (storage.Storage, error) {
	if st, err := LoadStorage(ct.storageURIExternal(), ct.encs); err != nil {
		return nil, err
	} else if err := st.Initialize(); err != nil {
		return nil, err
	} else {
		return st, nil
	}
}

func (ct *Container) Localstate() *isaac.Localstate {
	address, err := NewContestAddress(ct.name)
	if err != nil {
		panic(err)
	}
	st, err := ct.Storage()
	if err != nil {
		panic(err)
	}

	l, err := isaac.NewLocalstate(
		st,
		isaac.NewLocalNode(
			address,
			ct.privatekey,
		),
		ct.NodeDesign(false).NetworkID(),
	)
	if err != nil {
		panic(err)
	} else if err := l.Initialize(); err != nil {
		panic(err)
	}

	return l
}

func (ct *Container) Reset() {
	ct.setID("")
}
