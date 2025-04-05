# query.sh
#!/bin/bash

peer chaincode query -C mychannel -n lending -c '{"function":"GetLoan","Args":["LOAN001"]}'