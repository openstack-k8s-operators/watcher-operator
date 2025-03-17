#!/bin/bash
#
# Copyright 2025 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.
#

#
# Instructions to delete what is created in demo project
#
#  oc rsh openstackclient
#  unset OS_CLOUD
#  . ./demorc
#  openstack server delete test_0
#  If more that one were created, delete them
#  openstack router delete priv_router
#  openstack subnet delete priv_sub_demo
#  openstack network delete private_demo
#  openstack security group delete basic
#  openstack image delete cirros
#  export OS_CLOUD=default
#  openstack project delete demo
#


set -ex

# Create Image
IMG=cirros-0.5.2-x86_64-disk.img
URL=http://download.cirros-cloud.net/0.5.2/$IMG
DISK_FORMAT=qcow2
RAW=$IMG
NUMBER_OF_INSTANCES=${1:-1}

openstack project show demo || \
    openstack project create demo
openstack role add --user admin --project demo member

openstack network show public || openstack network create public --external --provider-network-type flat --provider-physical-network datacentre
openstack subnet show public_subnet || openstack subnet create public_subnet --subnet-range <PUBLIC SUBNET CIDR> --allocation-pool start=<START PUBLIC SUBNET ADDRESSES RANGE>,end=<END PUBLIC SUBNET ADDRESSES RANGE> --gateway <PUBLIC NETWORK GATEWAY IP> --dhcp --network public

# Create flavor
openstack flavor show m1.small || \
    openstack flavor create --ram 512 --vcpus 1 --disk 1 --ephemeral 1 m1.small

# Use the demo project from now on
unset OS_CLOUD
cp cloudrc demorc
sed -i 's/OS_PROJECT_NAME=admin/OS_PROJECT_NAME=demo/' demorc
. ./demorc

curl -L -# $URL > /tmp/$IMG
if type qemu-img >/dev/null 2>&1; then
    RAW=$(echo $IMG | sed s/img/raw/g)
    qemu-img convert -f qcow2 -O raw /tmp/$IMG /tmp/$RAW
    DISK_FORMAT=raw
fi

openstack image show cirros || \
    openstack image create --container-format bare --disk-format $DISK_FORMAT cirros < /tmp/$RAW

# Create networks
openstack network show private_demo || openstack network create private_demo
openstack subnet show priv_sub_demo || openstack subnet create priv_sub_demo --subnet-range 192.168.0.0/24 --network private_demo
openstack router show priv_router || {
    openstack router create priv_router
    openstack router add subnet priv_router priv_sub_demo
    openstack router set priv_router --external-gateway public
}

# Create security group and icmp/ssh rules
openstack security group show basic || {
    openstack security group create basic
    openstack security group rule create basic --protocol icmp --ingress --icmp-type -1
    openstack security group rule create basic --protocol tcp --ingress --dst-port 22
}

# Create an instance
for (( i=0; i<${NUMBER_OF_INSTANCES}; i++ )); do
    NAME=test_${i}
    openstack server show ${NAME} || {
        openstack server create --flavor m1.small --image cirros --nic net-id=private_demo ${NAME} --security-group basic --wait
        fip=$(openstack floating ip create public -f value -c floating_ip_address)
        openstack server add floating ip ${NAME} $fip
    }
    openstack server list --long

done
