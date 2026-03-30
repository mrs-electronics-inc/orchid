set dotenv-load

# Create a new VM from a repo: just create-vm <repo-url> [--name <name>]
create-vm +args:
    ./scripts/create-vm.sh {{args}}
