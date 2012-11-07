execute "apt-get update" do
  command "apt-get update"
end

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

  lib32gcc1

  git
  bzr
  curl
  lzop
  expect-dev

  mongodb
  ).each do |pkg|
  package pkg
end

%w(rake posix-spawn).each do |gem|
  gem_package gem
end

# install java
package 'python-software-properties'

execute "apt-get purge openjdk*" do
  command "apt-get purge openjdk*"
end

execute "add-apt-repository ppa:webupd8team/java" do
  command "add-apt-repository ppa:webupd8team/java"
end

execute "apt-get update" do
  command "apt-get update"
end

package 'oracle-java7-installer'