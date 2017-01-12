package virtualbox

type PortMode int

const (
	HOST_MODE_DISCONNECTED = 1 << iota
	HOST_MODE_PIPE
	HOST_MODE_DEVICE
	HOST_MODE_RAW_FILE
)

type SerialPort struct {
	HostFile   string
	PortNum    string
	IoBase     string
	Irq        string
	SType      PortMode
	ServerMode bool
}

func (m *Machine) SerialPortConf(hostFile, portNum, ioBase, irq string, sType PortMode, server bool) []string {
	var serverArg string
	switch sType {
	case HOST_MODE_DISCONNECTED, HOST_MODE_DEVICE:
		return []string{}
	case HOST_MODE_PIPE:
		if server {
			serverArg = "server"
		} else {
			serverArg = "client"
		}
	case HOST_MODE_RAW_FILE:
		serverArg = "file"
	}
	return []string{"--uart" + portNum, ioBase, irq, "--uartmode" + portNum, serverArg, hostFile}
}

func (m *Machine) CreateSerialPort(hostFile, portNum, ioBase, irq string, sType PortMode, server bool) error {
	conf := m.SerialPortConf(hostFile, portNum, ioBase, irq, sType, server)

	if len(conf) == 0 {
		return nil
	}

	args := append([]string{"modifyvm", m.Name}, conf...)
	if err := vbm(args...); err != nil {
		return err
	}
	return nil
}

func (m *Machine) StopSerialPort(portNum string) error {
	if err := vbm("modifyvm", m.Name, "--uart"+portNum, "off"); err != nil {
		return err
	}
	return nil
}
