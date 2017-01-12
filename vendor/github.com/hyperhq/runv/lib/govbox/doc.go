/*
Package virtualbox implements wrappers to interact with VirtualBox.

VirtualBox Machine State Transition

A VirtualBox machine can be in one of the following states:

	poweroff: The VM is powered off and no previous running state saved.
	running: The VM is running.
	paused: The VM is paused, but its state is not saved to disk. If you quit VirtualBox, the state will be lost.
	saved: The VM is powered off, and the previous state is saved on disk.
	aborted: The VM process crashed. This should happen very rarely.

VBoxManage supports the following transitions between states:

	startvm <VM>: poweroff|saved --> running
	controlvm <VM> pause: running --> paused
	controlvm <VM> resume: paused --> running
	controlvm <VM> savestate: running -> saved
	controlvm <VM> acpipowerbutton: running --> poweroff
	controlvm <VM> poweroff: running --> poweroff (unsafe)
	controlvm <VM> reset: running --> poweroff --> running (unsafe)

Poweroff and reset are unsafe because they will lose state and might corrupt
the disk image.

To make things simpler, the following transitions are used instead:

	 start: poweroff|saved|paused|aborted --> running
	 stop: [paused|saved -->] running --> poweroff
	 save: [paused -->] running --> saved
	 restart: [paused|saved -->] running --> poweroff --> running
	 poweroff: [paused|saved -->] running --> poweroff (unsafe)
	 reset: [paused|saved -->] running --> poweroff --> running (unsafe)

The takeaway is we try our best to transit the virtual machine into the state
you want it to be, and you only need to watch out for the potentially unsafe
poweroff and reset.

*/
package virtualbox
