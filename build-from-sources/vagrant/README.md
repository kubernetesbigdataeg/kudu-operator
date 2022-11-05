git clone https://github.com/apache/hive.git
git rev-parse HEAD -> a7280749056c4fb8b730975eb70190a175f67ef4
cat $(curl -LO https://patch-diff.githubusercontent.com/raw/apache/hive/pull/2326.patch) | patch -p1

https://github.com/apache/impala.git
git rev-parse HEAD -> fcaea30b151d89f412816a8e49d5feeef6964a0f


https://github.com/apache/kudu
git rev-parse HEAD -> 5fc302c0e4bcd977af51f0fa9a86d043b2263b55
kudu 1.16.0-SNAPSHOT


