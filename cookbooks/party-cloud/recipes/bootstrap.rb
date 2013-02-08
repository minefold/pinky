include_recipe 'party-cloud'

base_dir = '/usr/local/funpacks'
user = "vagrant"

base_url = 'https://party-cloud-production.s3.amazonaws.com/funpacks/slugs'
funpacks = [
  {
    :id => '50a976ec7aae5741bb000001',
    :url => "#{base_url}/minecraft/stable.tar.lzo"
  }, {
    :id => '50a976fb7aae5741bb000002',
    :url =>  "#{base_url}/minecraft-essentials/stable.tar.lzo"
  }, {
    :id => '50a977097aae5741bb000003',
    :url => "#{base_url}/tekkit/stable.tar.lzo"
  }, {
    :id => '50bec3967aae5797c0000004',
    :url => "#{base_url}/team-fortress-2/stable.tar.lzo"
  },
]

funpacks.each do |fp|
  %w(pack build).each do |dir|
    directory "#{base_dir}/#{fp[:id]}/#{dir}" do
      owner user
      mode 00755
      recursive true
    end
  end

  funpack_dir = "#{base_dir}/#{fp[:id]}"
  execute "download funpack #{fp[:id]}" do
    command %Q{
      export AWS_ACCESS_KEY=AKIAJPN5IJVEBB2QE35A
      export AWS_SECRET_KEY=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
      restore-dir #{fp[:url]}
    }
    cwd "#{funpack_dir}/pack"
  end

  execute "bootstrap funpack #{fp[:id]}" do
    cwd funpack_dir
    command "pack/bin/bootstrap build"
    only_if { File.exists?("pack/bin/bootstrap") }
  end
end
