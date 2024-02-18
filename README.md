# if-inject

Inject (and remove) interfaces in a running K8s POD

The default interface (`eth0`) is added to PODs by a `CNI-plugin`,
such as [kindnet](https://github.com/aojea/kindnet) or
[flannel](https://github.com/flannel-io/flannel). Extra interfaces can
be added with [Multus](https://github.com/k8snetworkplumbingwg/multus-cni).

It's a common belief that interfaces only can be added on POD startup
with CNI-plugins, or that it's very hard to add/remove them
dynamically. This repo shows that it's quite easy to add or remove
interfaces in running PODs using the [reference cni-plugins](
https://github.com/containernetworking/plugins) for example.

That said, it must be emphasised that maintaining such interfaces in
K8s, considering things like configuration, life-cycle management,
routing etc, can be *very* hard!

This purpose of this repo is educational, not to be a product. If you
need a product, please check the [Multus thick plugin](
https://github.com/k8snetworkplumbingwg/multus-cni), and monitor the
[K8s multi-network effort](
https://github.com/kubernetes/enhancements/pull/3700).



## Manually inject an interface

Interfaces can be moved into network namespaces with `ip link set
netns`.  K8s is no different in this regard, but it's difficult to
identify the netns. The `if-inject` program in this repo helps with
that, but you *must* execute the commands on the node where the POD is
running (since we must ask the local runtime, cri-o or containerd).

A `veth` pair is created and one side is moved into the POD.

```
make
# Copy "./if-inject" to a node 
kubectl create namespace if-inject
kubectl create -n if-inject -f test/alpine.yaml
kubectl get pods -n if-inject -o wide
# On the node, pick a local POD
pod=<pod-on-the-local-node>
if-inject netns -ns if-inject -pod $pod
# Output with cri-o (1.28.1):
/var/run/netns/c913ee97-ebf8-4e77-8167-96b157e5e149
# Output with containerd (1.7.11):
/proc/776/ns/net

ns=$(if-inject netns -ns if-inject -pod $pod)
ip link add pod0 type veth peer net1
ip link set net1 netns $ns
kubectl exec -n if-inject $pod -- ip link set net1 up
ip link set pod0 up
kubectl exec -n if-inject $pod -- ip addr show net1
ping fe80::48c8:baff:fed4:32d0%pod0   # (use your own link-local address of course)
```

Since a `veth` pair is used, you can clean-up by removing either interface:
```
kubectl exec -n if-inject $pod -- ip link del net1
# Or
ip link del pod0
```


## Inject an interface with a CNI-plugin

We will do the same thing as in the previous paragraph but using the
[ptp cni-plugin](https://www.cni.dev/plugins/current/main/ptp/). The
`ptp` and `host-local` plugins must be in `/opt/cni/bin/` on the node.

```
# Copy "./if-inject" and "test/ptp.json" to a node 
kubectl create namespace if-inject
kubectl create -n if-inject -f test/alpine.yaml
kubectl get pods -n if-inject -o wide
# On the node, pick a local POD
pod=<pod-on-the-local-node>
if-inject add -ns if-inject -pod $pod -spec ptp.json 2>&1 | jq
kubectl exec -n if-inject $pod -- ip addr show net1
ping 10.22.22.2 # (use your own if it differs)
```

The `ptp` plugin *requires* an "ipam", so this time an IPv4 address is
assigned. Remove with:

```
if-inject del -ns if-inject -pod $pod -spec ptp.json
```

## The if-inject program

The `if-inject` program is "inspired" by the [cnitool](
https://github.com/containernetworking/cni/tree/main/cnitool), but is
adapted to K8s in the sense that you specify the K8s namespace and
POD-name rather then the Linux network namespace.

Build:
```
__version=0.0.1-local make O=/tmp   # O is the destination dir
/tmp/if-inject -version
```

Logging goes to `stderr` and is in `json` format. Example:

```
if-inject -loglevel 2 netns -ns if-inject -pod $pod 2> log
cat log | jq
```

Use a go workspace to use a local libcni:
```
go work init
go work use .
go work use $GOPATH/src/github.com/containernetworking/cni
./build.sh static   # (no src files are altered so "make" will not rebuild)
```

