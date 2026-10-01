dependency "network" {
  config_path = "../network"
}

inputs = {
  vpc_id = dependency.network.outputs.vpc_id
}
