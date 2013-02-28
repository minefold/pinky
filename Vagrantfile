Vagrant::Config.run do |config|
  config.vm.box = "base"

  config.vm.network :hostonly, "10.10.10.15"

  config.vm.customize ["modifyvm", :id, "--memory", 1024 * 2]

  # config.vm.boot_mode = :gui

  (10000..12000).each do |port|
    config.vm.forward_port port, port
    config.vm.forward_port port, port, protocol: 'udp'
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
