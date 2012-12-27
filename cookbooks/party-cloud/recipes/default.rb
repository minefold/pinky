
install_dir = "/usr/local/party-cloud"

directory "#{install_dir}/bin" do
  owner "root"
  mode 00755
  recursive true
  action    :create
end

%w(s3curl restore-dir).each do |binary|
  cookbook_file "#{install_dir}/bin/#{binary}" do
    source binary
    mode 00755
  end

  execute "symlink #{binary}" do
    command "ln -sf #{install_dir}/bin/#{binary} /usr/bin/#{binary}"
  end
end