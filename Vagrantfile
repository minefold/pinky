require 'berkshelf/vagrant'

Vagrant::Config.run do |config|
  config.vm.box = "quantal"
  config.vm.host_name = "pinky"

  config.vm.network :hostonly, "10.10.10.15"

  config.vm.customize ["modifyvm", :id, "--memory", 1024 * 2]

  # config.vm.boot_mode = :gui

  (10000..12000).each do |port|
    config.vm.forward_port port, port
    config.vm.forward_port port, port, protocol: 'udp'
  end

  config.vm.provision :chef_solo do |chef|
    chef.json = {}

    chef.run_list = [
      "recipe[apt]",
      "recipe[main]",
      "recipe[party-cloud]",
      "recipe[party-cloud::bootstrap]",
    ]
  end
end
