set dotenv-load

# Create a new VM: just create-vm <name> [--packages "pkg1 pkg2"]
create-vm +args:
    ./scripts/create-vm.sh {{args}}
