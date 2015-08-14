package dockerversion

var (
	GITCOMMIT string = "$GITCOMMIT"
	VERSION   string = "1.6.2"
	BUILDTIME string = "$BUILDTIME"
	IAMSTATIC string = "${IAMSTATIC:-true}"
	INITSHA1  string = "$DOCKER_INITSHA1"
	INITPATH  string = "$DOCKER_INITPATH"
)
