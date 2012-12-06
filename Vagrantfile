Vagrant::Config.run do |config|
  config.vm.box = "base"

  config.vm.network :hostonly, "10.10.10.15"

  config.vm.customize ["modifyvm", :id, "--memory", 2048]
  
  [20000, 27015, 28015].each do |port|
    config.vm.forward_port port, port
    config.vm.forward_port port, port, protocol: 'udp'
  end

  config.vm.provision :chef_solo do |chef|
    # chef.cookbooks_path = [
    #   "cookbooks", 
    #   "~/code/minefold/cookbooks"]

    chef.add_recipe "main"
    chef.add_recipe "golang"
    # chef.add_recipe "java"
  end
  
end
