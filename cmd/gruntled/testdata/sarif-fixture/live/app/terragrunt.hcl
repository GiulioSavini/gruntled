dependency "network" {
  config_path = "../network"
}

dependency "db" {
  config_path = "../db"
}

inputs = {
  vpc_id = dependency.network.outputs.vpc_idd
}
