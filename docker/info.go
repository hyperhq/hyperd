package docker

func (cli *DockerCli) SendCmdInfo(args ...string) ([]byte, int, error){
	return readBody(cli.Call("GET", "/info", nil, nil))
}
