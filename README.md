[![Build Status](https://travis-ci.org/hyperhq/hyper.svg?branch=master)](https://travis-ci.org/hyperhq/hyper)
Hyper - Hypervisor-agnostic Docker Runtime
====

## What is Hyper?

**Hyper is a hypervisor-agnostic tool that allows you to run Docker images on any hypervisor**.

![](docs/static_files/hyper-run.png)

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
- Xen 4.5 and VT enabled host (for Xen support)

## Installation

Ensure you are running Linux (kernel 3.8 or later) and have Docker
(version 1.5 or later) and QEMU (version 2.0 or later) installed. Then install hyper with

    curl -sSL https://hyper.sh/install | bash

Or download the [binary tarball](https://hyper-install.s3.amazonaws.com/hyper-latest.tgz) and install it directly.

To run *hyper*, just type `hyper` if you've installed packages.

For information on using the command line, just type `hyper`. You may use
`hyper <command> --help` for detailed information on any specific command.


## Example


## Build From Source

Clone hyper and runv in GoPath

    > mkdir -p ${GOPATH}/src/github.com/hyperhq
    > cd ${GOPATH}/src/github.com/hyperhq
	> git clone https://github.com/hyperhq/runv.git runv
	> git clone https://github.com/hyperhq/hyper.git hyper

And make sure you have `go` (>= 1.4) , `device-mapper-devel`, and `autotools`, go into the `hyper` dir

    > ./autogen.sh
    > ./configure
    > make

Then you can get the binaries `hyperd` daemon and `hyper` cmdline tool.

You may also need the kernel and initrd from [HyperStart](https://github.com/hyperhq/hyperstart) to run your own hyper.


## Find out more

 * [Documentation](https://docs.hyper.sh)
 * [Get Started](https://docs.hyper.sh/get_started/index.html)
 * [Reference](https://docs.hyper.sh/reference/index.html)
 * [Release Notes](https://docs.hyper.sh/release_notes/latest.html)

## Contact Us

Found a bug, want to suggest a feature, or have a question?
[File an issue](https://github.com/hyperhq/hyper/issues), or email <bug@hyper.sh>. When reporting a bug, please include which version of
hyper you are running, as shown by `hyper --version`.

* Twitter: [@hyper_sh](https://twitter.com/hyper_sh)
* Blog: [https://hyper.sh/blog.html](https://hyper.sh/blog.html)
* Slack: [#hyper](https://slack.hyper.sh/) (The IRC has been migrated to slack.)
