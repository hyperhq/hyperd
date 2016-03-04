package client

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hyperhq/hyper/engine"

	gflag "github.com/jessevdk/go-flags"
)

func (cli *HyperClient) HyperCmdImages(args ...string) error {
	var opts struct {
		All bool `short:"a" long:"all" default:"false" value-name:"false" description:"Show all images (by default filter out the intermediate image layers)"`
		Num bool `short:"q" long:"quiet" default:"false" value-name:"false" description:"Only show numeric IDs"`
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
	v := url.Values{}
	v.Set("all", "no")
	v.Set("quiet", "no")
	if opts.All == true {
		v.Set("all", "yes")
	}
	if opts.Num == true {
		v.Set("quiet", "yes")
	}
	body, _, err := readBody(cli.call("GET", "/images/get?"+v.Encode(), nil, nil))
	if err != nil {
		return err
	}
	out := engine.NewOutput()
	remoteInfo, err := out.AddEnv()
	if err != nil {
		return err
	}

	if _, err := out.Write(body); err != nil {
		fmt.Printf("Error reading remote info: %s", err)
		return err
	}
	out.Close()

	var (
		imagesList = []string{}
	)
	imagesList = remoteInfo.GetList("imagesList")

	w := tabwriter.NewWriter(cli.out, 20, 1, 3, ' ', 0)
	if opts.Num == false {
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
	var rtn float64
	if s < 1024 {
		return size + " B"
	} else if s < 1024*1024 {
		rtn = float64(s) / 1024.0
		return fmt.Sprintf("%.1f KB", rtn)
	} else {
		rtn = float64(s) / (1024.0 * 1024.0)
		return fmt.Sprintf("%.1f MB", rtn)
	}
}
