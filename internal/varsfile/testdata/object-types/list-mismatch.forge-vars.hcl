# exposed_ports declared as list(number); supplying a string element here
# (and a number) forces cty.Convert to fail because the element type
# can't unify cleanly to a single declared type.
exposed_ports = ["http", 9090]
