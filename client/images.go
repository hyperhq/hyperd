package client

import (
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/docker/go-units"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdImages(args ...string) error {
	var opts struct {
		All   bool `short:"a" long:"all" default:"false" description:"Show all images (by default filter out the intermediate image layers)"`
		Quiet bool `short:"q" long:"quiet" default:"false" description:"Only show numeric IDs"`
	}
	var parser = gflag.NewParser(&opts, gflag.Default)

	parser.Usage = "images [OPTIONS] [REPOSITORY]\n\nList images"
	args, err := parser.ParseArgs(args)
	if err != nil {
		if !strings.Contains(err.Error(), "Usage") {
			return err
		} else {
			return nil
		}
	}

	remoteInfo, err := cli.client.GetImages(opts.All, opts.Quiet)
	if err != nil {
		return err
	}

	var (
		imagesList = []string{}
	)
	imagesList = remoteInfo.GetList("imagesList")

	w := tabwriter.NewWriter(cli.out, 20, 1, 3, ' ', 0)
	if opts.Quiet == false {
		fmt.Fprintln(w, "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tVIRTUAL SIZE")
		for _, item := range imagesList {
			fields := strings.Split(item, ":")
			date, _ := strconv.ParseUint(fields[3], 0, 64)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", fields[0], fields[1], fields[2][:12], time.Unix(int64(date), 0).Format("2006-01-02 15:04:05"), getImageSizeString(fields[4]))
		}
	} else {
		for _, item := range imagesList {
			fields := strings.Split(item, ":")
			fmt.Fprintf(w, "%s\n", fields[2][:12])
		}
	}
	w.Flush()

	return nil
}

func getImageSizeString(size string) string {
	s, err := strconv.Atoi(size)
	if err != nil {
		return ""
	}

	rtn := float64(s)
	return units.HumanSize(rtn)
}
