## VirtualBox implement tips

### Volume
* Use the vdi created by `VboxManage` if the user do not specific the volume directory, such as

    Create a 32GB “dynamic” disk:
    
    ```
    VBoxManage createhd --filename volume.vdi --size 32768
    ```
    And attach it to the VM:
    
    ```
     VBoxManage storageattach $VM --storagectl "SATA Controller" \
                --port 0 --device 0 --type hdd --medium volume.vdi
    ```


    ```
    Attention:
    There is no any interface to create a new vdi in **go-virtualbox** library, need to patch it.
    ```
* Use the shared folder to attach a volume if the user specific the volume directory, such as 
   
   
   ```
   VBoxManage sharedfolder add add <uuid|vmname> --name <name> --hostpath <hostpath>
   ```
   
### Image
* `VirtualBox` boot from a iso image, which includes kernel and initrd.img
* Not sure the boot time, we may focus the boot successfully, not performance


### Serial Port
* Use `ttyS0` and `ttyS1` to communicate between host and guest via socket, since `VirtualBox` only support two serial ports
* Set the serial port to use `Host Pipe` of `VirtualBox`
    
>
The following other hardware settings are available through `VBoxManage` modifyvm:
>
>
###### --uart<1-N> off|<I/O base> <IRQ>: 
    With this option you can configure virtual serial ports for the VM.
>
###### --uartmode<1-N> arg: 
    This setting controls how VirtualBox connects a given virtual serial port (previously configured with the --uartX setting, see above) to the host on which the virtual machine is running. As described in detail in Section 3.9, “Serial ports”, for each such port, you can specify <arg> as one of the following options:
>
    * disconnected: Even though the serial port is shown to the guest, it has no "other end" -- like a real COM port without a cable.
>
    * server <pipename>: On a Windows host, this tells VirtualBox to create a named pipe on the host named <pipename> and connect the virtual serial device to it. Note that Windows requires that the name of a named pipe begin with \\.\pipe\.
>
    On a Linux host, instead of a named pipe, a local domain socket is used.
>
    * client <pipename>: This operates just like server ..., except that the pipe (or local domain socket) is not created by VirtualBox, but assumed to exist already.
>
    * <devicename>: If, instead of the above, the device name of a physical hardware serial port of the host is specified, the virtual serial port is connected to that hardware port. On a Windows host, the device name will be a COM port such as COM1; on a Linux host, the device name will look like /dev/ttyS0. This allows you to "wire" a real serial port to a virtual machine.

```
Attention:
This interface is not implemented in go-virtualbox library
```

