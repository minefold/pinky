Vagrant.configure("2") do |config|
  config.vm.box = "quantal"
  config.vm.box_url = "https://github.com/downloads/roderik/VagrantQuantal64Box/quantal64.box"

  config.vm.network :private_network, ip: "10.10.10.15"

  config.vm.provider :virtualbox do |vb|
    vb.customize ["modifyvm", :id, "--memory", 1024 * 2]
  end

  # config.vm.boot_mode = :gui

  (10000..12000).each do |port|
    config.vm.network :forwarded_port, guest: port, host: port, protocol: 'udp'
    config.vm.network :forwarded_port, guest: port, host: port, protocol: 'tcp'
  end

  config.vm.provision :chef_solo do |chef|
    # chef.cookbooks_path = [
    #   "cookbooks",
    #   "~/code/minefold/cookbooks"]

    chef.add_recipe "main"
    # chef.add_recipe "golang"
    # chef.add_recipe "java"
    chef.add_recipe "party-cloud"
    chef.add_recipe "party-cloud::bootstrap"
  end

end
