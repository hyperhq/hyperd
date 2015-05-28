
Hyper - Hypervisor-agnostic Docker Runtime
====

## What is Hyper?

**Hyper is a hypervisor-agnostic tool that allows you to run Docker images on any hypervisor**.

![](https://trello-attachments.s3.amazonaws.com/5551c49246960a31feab3d35/696x332/cf0bd3c0f795c5dc53dde2c5cb51d6ba/hyper_cli.png)

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

Requirements
------------

*Hyper* requires at least go 1.1 in order to build and at least Linux 3.8, with appropriate headers. If your headers are not recent enough, you may see some compilation errors related to
`KVM_SIGNAL_MSI` or `KVM_CAP_SIGNAL_MSI`.

Building
---------

To build *hyper*, simply clone the repo and run `make`.

Optionally, you can build packages by running `make deb` or `make rpm`.

To run *hyper*, use `bin/hyper`, or if you've installed packages, just type `hyper`.

For information on using the command line, just type `hyper`. You may use
`hyper <command> --help` for detailed information on any specific command.

To create a instance, try using `hyper run --com1 /bin/ls`.


## Installation

Ensure you are running Linux (kernel 3.8 or later) and have Docker
(version 1.5 or later) and QEMU (version 2.0 or later) installed. Then install hyper with

    sudo curl https://install.hyper.sh | sh

## Example

## Find out more

 * [Documentation](https://docs.hyper.sh)
 * [Get Started](https://docs.hyper.sh/get_started/index.html)
 * [Reference](https://docs.hyper.sh/reference/index.html)
 * [Release Notes](https://docs.hyper.sh/release_notes/latest.html)

## Contact Us

Found a bug, want to suggest a feature, or have a question?
[File an issue](https://github.com/hyperhq/hyper/issues), or email [bug@hyper.sh](bug@hyper.sh). When reporting a bug, please include which version of
hyper you are running, as shown by `hyper --version`.

Twitter: [@hyperhq](https://twitter.com/hyper_sh)
Blog: [https://hyper.sh/blog](https://hyper.sh/blog)
IRC: [#hyper](https://botbot.me/freenode/hyper/)

