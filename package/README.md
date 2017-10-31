## Build Binary Packages for Distros

4 Distros are supported by HyperContainer binary pacaking tools currently.
The `Dockerfile` of builders are hosted in repo [hyperhq/official-images](https://github.com/hyperhq/official-images):

- CentOS: [centos_builder_2](https://github.com/hyperhq/official-images/tree/master/centos_builder_2) ;
- Fedora: [fedora_builder_2](https://github.com/hyperhq/official-images/tree/master/fedora_builder_2) ;
- Debian: [debian_builder_2](https://github.com/hyperhq/official-images/tree/master/debian_builder_2) ;
- Ubuntu: [centos_builder_2](https://github.com/hyperhq/official-images/tree/master/ubuntu_builder_2) ;

All these images shared the same usages, for example, the image name is `gnawux/buildenv:centos` here:

```bash
docker run -it -e HYPERD_REF=pull/563/merge -e UPLOAD=upload/rc1 -e AWS_ACCESSKEY=AAAAAA -e AWS_SECRETKEY=fffffff+ppppp/k gnawux/buildenv:centos

```

#### Supported Environments

The default environments of the images are defined as:

```bash
HYPERD_REF=${HYPERD_REF:-heads/master}
HYPERSTART_REF=${HYPERSTART_REF:-heads/master}
BUILD=${BUILD:-yes}
UPLOAD=${UPLOAD:-none}
ACCESS=${AWS_ACCESSKEY:-none}
SECRET=${AWS_SECRETKEY:-none}
```

- `HYPERD_REF` : The hyperd git reference to be built, master by default, and if you want to build master with PR#563 merged, then specify `-e HYPERD_REF=pull/563/merge`
- `HYPERSTART_REF`: The hyperstart git reference to be built, similar to hyperd.
- `BUILD`: flag for build, won't build if not yes, default is `yes`.
- `UPLOAD`: the S3 bucket (path) will be uploaded to, won't upload if not specify, for example `candidates/rc-1`.
- `ACCESS` and `SECRET`: AWS credential for package uploading, the credential should have the permission to upload to the path specify by `UPLOAD`.
