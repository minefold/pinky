include_recipe 'party-cloud'

base_dir = '/usr/local/funpacks'
user = "vagrant"

base_url = 'https://party-cloud-production.s3.amazonaws.com/funpacks'
funpacks = [{
    :id => '50bec3967aae5797c0000004',
    :pack => "#{base_url}/slugs/50bec3967aae5797c0000004.tar.gz",
    :shared => "#{base_url}/shared/50bec3967aae5797c0000004.tar.lzo"
  },
]

funpacks.each do |fp|
  %w(pack shared).each do |dir|
    directory "#{base_dir}/#{fp[:id]}/#{dir}" do
      owner user
      mode 00755
      recursive true
    end
  end

  funpack_dir = "#{base_dir}/#{fp[:id]}"
  execute "download pack #{fp[:id]}" do
    command %Q{
      export AWS_ACCESS_KEY=AKIAJPN5IJVEBB2QE35A
      export AWS_SECRET_KEY=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
      restore-dir #{fp[:pack]}
    }
    cwd "#{funpack_dir}/pack"
  end

  # execute "download shared #{fp[:id]}" do
  #   command %Q{
  #     export AWS_ACCESS_KEY=AKIAJPN5IJVEBB2QE35A
  #     export AWS_SECRET_KEY=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
  #     restore-dir #{fp[:shared]}
  #   }
  #   cwd "#{funpack_dir}/shared"
  # end

  execute "bootstrap funpack #{fp[:id]}" do
    cwd funpack_dir
    command "pack/bin/bootstrap shared"
    only_if { File.exists?("pack/bin/bootstrap") }
  end
end
