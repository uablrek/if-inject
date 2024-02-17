# if-inject

Inject (and remove) interfaces in a running K8s POD

The default interface (`eth0`) is added to PODs by a `CNI-plugin`,
such as [kindnet](https://github.com/aojea/kindnet) or
[flannel](https://github.com/flannel-io/flannel). Extra interfaces can
be added with [Multus](https://github.com/k8snetworkplumbingwg/multus-cni).

It's a common belief that interfaces only can be added on POD startup
in K8s, or that it's very hard to add/remove them dynamically. This
repo shows that it's quite easy to add or remove interfaces in running
PODs using the [reference cni-plugins](
https://github.com/containernetworking/plugins) for example.

That said, it must be emphasised that maintaining such interfaces in
K8s, considering things like configuration, life-cycle management,
routing etc, is *very* hard!


## Manually inject an interface

Interfaces can be moved into network namespaces with `ip link set
netns`.  K8s is no different in this regard, but it's difficult to
identify the netns. The `if-inject` program in this repo helps with
that, but you *must* execute the commands on the node where the POD is
running (since we must ask the local runtime, cri-o or containerd).

This is educational rather than useful. A `veth` pair is created and
one side is moved into the POD.

```
make
# Copy "./if-inject" to a node 
kubectl create namespace if-inject
kubectl create -n if-inject -f test/alpine.yaml
kubectl get pods -n if-inject -o wide
# On the node, pick a local POD
pod=alpine-84bc57864d-vfg9p
if-inject getnetns -ns if-inject -pod $pod
# Output with cri-o (1.28.1):
/var/run/netns/c913ee97-ebf8-4e77-8167-96b157e5e149
# Output with containerd (1.7.11):
/proc/776/ns/net

ns=$(if-inject getnetns -ns if-inject -pod $pod)
ip link add pod0 type veth peer net1
ip link set net1 netns $ns
kubectl exec -n if-inject $pod -- ip link show net1
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
