#
# Cookbook Name:: redis
# Recipe:: _server_install_from_package

package "redis" do
  package_name "redis-server"
  action :install
end
