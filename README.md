
Hyper - Hypervisor-agnostic Docker Runtime
====

## What is Hyper?

**Hyper is a hypervisor-agnostic tool that allows you to run Docker images on any hypervisor**.

![](https://trello-attachments.s3.amazonaws.com/5551c49246960a31feab3d35/508x224/3b4cc5009489f917b7394a40b6cb57ec/upload_5_29_2015_at_12_56_51_AM.png)

## Why Hyper?
-----------

**Hyper combines the best from both world: VM and Container**.

| -  | Container | VM | Hyper | 
|---|---|---|---|
| Isolation | Weak, shared kernel | Strong, HW-enforced  | Strong, HW-enforced  |
| Portable  | Yes, but kernel dependent sometimes | No, hypervisor dependent | Yes, hypervisor agnostic and portable image |
| Boot  | Fast, sub-second  | Slow, tens of seconds  | Fast, sub-second  |
| Performance  | Great | OK| Good, minimal resource footprint and overhead |
| Immutable | Yes  | No, configuration management required | Yes, only kernel+image  | 
| Image Size| Small, MBs  | Big, GBs  | Small, MBs  |
| Compatibility | No, need new tools | Great, everything just works  | Good, it is still a "Machine", much less changes  |
| Mature   | Not yet  | Production ready, SDN, SDS, LiveMigration, etc.  | Yes, just plug-&-play|
| ROI| Rebuild everything with container  | - | Reuse your virtual infrastructure  |

> **BYOK* = bring your own kernel

## Requirements

- Docker 1.5 or later
- QEMU 2.0 or later

## Installation

Ensure you are running Linux (kernel 3.8 or later) and have Docker
(version 1.5 or later) and QEMU (version 2.0 or later) installed. Then install hyper with

    curl -sSL https://hyper.sh/install | bash

To run *hyper*, just type `hyper` if you've installed packages.

For information on using the command line, just type `hyper`. You may use
`hyper <command> --help` for detailed information on any specific command.


## Example


## Build From Source

Clone hyper in GoPath

    > cd ${GOPATH}/src
	> git clone https://github.com/hyperhq/hyper.git hyper

Makesure some dependency go packages installed

    > cd hyper
    > ./make_deps.sh

And got hyper binaries with `go build`

    > go build hyperd.go
    > go build hyper.go

You may also need the kernel and initrd from [HyperStart](https://github.com/hyperhq/hyperstart) to run your own hyper.


## Find out more

 * [Documentation](https://docs.hyper.sh)
 * [Get Started](https://docs.hyper.sh/get_started/index.html)
 * [Reference](https://docs.hyper.sh/reference/index.html)
 * [Release Notes](https://docs.hyper.sh/release_notes/latest.html)

## Contact Us

Found a bug, want to suggest a feature, or have a question?
[File an issue](https://github.com/hyperhq/hyper/issues), or email [bug@hyper.sh](bug@hyper.sh). When reporting a bug, please include which version of
hyper you are running, as shown by `hyper --version`.

Twitter: [@hyper_sh](https://twitter.com/hyper_sh)
Blog: [https://hyper.sh/blog](https://hyper.sh/blog.html)
IRC: [#hyperhq](http://webchat.freenode.net?channels=hyper&uio=d4) on freenode
