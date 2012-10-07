Vagrant::Config.run do |config|
  config.vm.box = "base"

  [27015, 28015].each do |port|
    config.vm.forward_port port, port
    config.vm.forward_port port, port, protocol: 'udp'
  end

  config.vm.provision :chef_solo do |chef|
    # chef.cookbooks_path = [
    #   "cookbooks", 
    #   "~/code/minefold/cookbooks"]

    chef.add_recipe "main"
    chef.add_recipe "golang"
    chef.add_recipe "redis::server"
    # chef.add_recipe "java"
  end

  # share TF2 funpack
  config.vm.share_folder "team-fortress-2.funpack",
    "~/funpacks/team-fortress-2.funpack",
    "../funpacks/team-fortress-2.funpack/build"

  # share minecraft-essentials funpack
  config.vm.share_folder "minecraft-essentials",
    "~/funpacks/minecraft-essentials",
    "../funpacks/minecraft-essentials"
  
end
