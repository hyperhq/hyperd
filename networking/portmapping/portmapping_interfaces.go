package portmapping

func SetupPortMaps(containerip string, externalPrefix []string, maps []*PortMapping) (preExec [][]string, err error) {
	if len(maps) == 0 {
		return [][]string{}, nil
	}
	if len(externalPrefix) > 0 {
		preExec, err = setupInSandboxMappings(externalPrefix, maps)
		if err != nil {
			return [][]string{}, err
		}
		defer func() {
			if err != nil {
				preExec, _ = releaseInSandboxMappings(externalPrefix, maps)
			}
		}()
	}
	if !disableIptables {
		err = setupIptablesPortMaps(containerip, maps)
		if err != nil {
			return [][]string{}, err
		}
	}
	return preExec, nil
}

func ReleasePortMaps(containerip string, externalPrefix []string, maps []*PortMapping) (postExec [][]string, err error) {
	if len(maps) == 0 {
		return [][]string{}, nil
	}
	if len(externalPrefix) > 0 {
		postExec, err = releaseInSandboxMappings(externalPrefix, maps)
		if err != nil {
			return [][]string{}, err
		}
	}
	if !disableIptables {
		err = releaseIptablesPortMaps(containerip, maps)
		if err != nil {
			return [][]string{}, err
		}
	}
	return postExec, nil
}
