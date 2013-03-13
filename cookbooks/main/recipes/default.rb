%w(
  build-essential
  ruby1.9.1
  ruby1.9.1-dev
  rubygems1.9.1
  irb1.9.1
  ri1.9.1
  rdoc1.9.1
  libopenssl-ruby1.9.1
  libssl-dev
  zlib1g-dev

  ntp

  lib32gcc1

  git
  bzr
  curl
  lzop
  expect-dev

  golang-go
  lxc
  ).each do |pkg|
  package pkg
end

%w(rake posix-spawn bundler).each do |gem|
  gem_package gem
end

# go
execute "chown go dir" do
  command "chown -RL vagrant /usr/lib/go"
end

# install java
package 'python-software-properties'

execute "apt-get purge openjdk*" do
  command "apt-get purge openjdk*"
  not_if 'java -version 2>&1 | grep 1.7'
end

execute "add-apt-repository ppa:webupd8team/java" do
  command "add-apt-repository -y ppa:webupd8team/java && apt-get update"
  not_if 'java -version 2>&1 | grep 1.7'
end

package 'oracle-java7-installer'

execute "create build path" do
  command %Q{
    mkdir -p /opt/funpacks
    chown vagrant /opt/funpacks
  }
end

template "/etc/profile.d/configurator.sh" do
  owner  'root'
  group  'root'
  mode   0644
  variables node[:main]
end

