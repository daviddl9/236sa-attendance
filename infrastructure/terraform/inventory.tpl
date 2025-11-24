[attendance]
${host_ip} ansible_user=ubuntu ansible_ssh_private_key_file=${ssh_key} ansible_python_interpreter=/usr/bin/python3

[attendance:vars]
ansible_ssh_common_args='-o StrictHostKeyChecking=no'
