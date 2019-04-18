对象池的多种实现测试
1. Lock+Slice
2. Lock+List
3. channel
4. 分桶+多个channel
5. LockFreeList
6. 分桶+多个LockFreeList
7. LockFreeSlice
8. 分桶+LockFreeSlice
9. 原生sync.Pool
10. 分桶+原生sync.Pool

Note: 原生sync.Pool只是用来做对比测试, 之前的实际实验中已经确认他没有收益(#9396), 因为GC时原生对象池内的对象会被回收.

Note: 测试时设置并行数为CPU核心数, 然后进行并发测试.

Note: 分桶都分为128个桶.

结论:
1. LockFree的实现和sync.Pool性能已经较为接近.
2. 并发数越大, 分桶策略优势约明显.
3. 无并发时, 策略3效果最好.
4. 并发大于等于4时, 策略4, 6, 8的效果就超过3了.
5. 在策略4, 6, 8中, 性能从好到次的顺序为6 > 8 > 4.

下面是测试数据:

1: 在本地MAC
```
机器配置:
hw.ncpu: 4
hw.byteorder: 1234
hw.memsize: 8589934592
hw.activecpu: 4
hw.physicalcpu: 2
hw.physicalcpu_max: 2
hw.logicalcpu: 4
hw.logicalcpu_max: 4
hw.cputype: 7
hw.cpusubtype: 8
hw.cpu64bit_capable: 1
hw.cpufamily: 260141638
hw.cacheconfig: 4 2 2 4 0 0 0 0 0 0
hw.cachesize: 8589934592 32768 262144 4194304 0 0 0 0 0 0
hw.pagesize: 4096
hw.pagesize32: 4096
hw.busfrequency: 100000000
hw.busfrequency_min: 100000000
hw.busfrequency_max: 100000000
hw.cpufrequency: 2300000000
hw.cpufrequency_min: 2300000000
hw.cpufrequency_max: 2300000000
hw.cachelinesize: 64
hw.l1icachesize: 32768
hw.l1dcachesize: 32768
hw.l2cachesize: 262144
hw.l3cachesize: 4194304


测试结果:
NUM CPU:  4
goos: darwin
goarch: amd64
pkg: lab/pool
BenchmarkLockSlice-4                    10000000               226 ns/op
BenchmarkLockList-4                      5000000               283 ns/op
BenchmarkChannel-4                      20000000                87.4 ns/op
BenchmarkMultiChannel-4                 20000000                89.1 ns/op
BenchmarkLockFreeList-4                 10000000               130 ns/op
BenchmarkMultiLockFreeList-4            20000000                80.4 ns/op
BenchmarkLockFreeSlice-4                10000000               132 ns/op
BenchmarkMultiLockFreeSlice-4           20000000                81.6 ns/op
BenchmarkSyncPool-4                     10000000               135 ns/op
BenchmarkMultiSyncPool-4                20000000                74.0 ns/op
BenchmarkCASUnsafe-4                    200000000                9.40 ns/op
BenchmarkCASInt64-4                     200000000                7.03 ns/op
```

2: 开发机1
```
机器配置:
Architecture:          x86_64
CPU op-mode(s):        32-bit, 64-bit
Byte Order:            Little Endian
CPU(s):                8
On-line CPU(s) list:   0-7
Thread(s) per core:    2
Core(s) per socket:    4
座：                 1
NUMA 节点：         1
厂商 ID：           GenuineIntel
CPU 系列：          6
型号：              60
型号名称：        Intel(R) Core(TM) i7-4790 CPU @ 3.60GHz
步进：              3
CPU MHz：             800.024
CPU max MHz:           4000.0000
CPU min MHz:           800.0000
BogoMIPS：            7183.22
虚拟化：           VT-x
L1d 缓存：          32K
L1i 缓存：          32K
L2 缓存：           256K
L3 缓存：           8192K

测试结果:
NUM CPU:  8
NUM CPU:  8
goos: linux
goarch: amd64
BenchmarkLockSlice-8            	 5000000	       368 ns/op
BenchmarkLockList-8             	 3000000	       512 ns/op
BenchmarkChannel-8              	 5000000	       297 ns/op
BenchmarkMultiChannel-8         	20000000	        73.4 ns/op
BenchmarkLockFreeList-8         	10000000	       162 ns/op
BenchmarkMultiLockFreeList-8    	20000000	        62.2 ns/op
BenchmarkLockFreeSlice-8        	10000000	       181 ns/op
BenchmarkMultiLockFreeSlice-8   	20000000	        73.4 ns/op
BenchmarkSyncPool-8             	10000000	       239 ns/op
BenchmarkMultiSyncPool-8        	20000000	        63.2 ns/op
BenchmarkCASUnsafe-8            	200000000	         8.78 ns/op
BenchmarkCASInt64-8             	300000000	         4.79 ns/op
```

3: 开发机2
```
机器配置:
Architecture:          x86_64
CPU op-mode(s):        32-bit, 64-bit
Byte Order:            Little Endian
CPU(s):                40
On-line CPU(s) list:   0-39
Thread(s) per core:    2
Core(s) per socket:    10
Socket(s):             2
NUMA node(s):          2
Vendor ID:             GenuineIntel
CPU family:            6
Model:                 79
Model name:            Intel(R) Xeon(R) CPU E5-2630 v4 @ 2.20GHz
Stepping:              1
CPU MHz:               2200.159
CPU max MHz:           3100.0000
CPU min MHz:           1200.0000
BogoMIPS:              4400.31
Virtualization:        VT-x
L1d cache:             32K
L1i cache:             32K
L2 cache:              256K
L3 cache:              25600K
NUMA node0 CPU(s):     0,2,4,6,8,10,12,14,16,18,20,22,24,26,28,30,32,34,36,38
NUMA node1 CPU(s):     1,3,5,7,9,11,13,15,17,19,21,23,25,27,29,31,33,35,37,39

测试结果:
NUM CPU:  40
goos: linux
goarch: amd64
BenchmarkLockSlice-40             	 3000000	       599 ns/op
BenchmarkLockList-40              	 2000000	      1082 ns/op
BenchmarkChannel-40               	 1000000	      1173 ns/op
BenchmarkMultiChannel-40          	10000000	       218 ns/op
BenchmarkLockFreeList-40          	 3000000	       651 ns/op
BenchmarkMultiLockFreeList-40     	10000000	       214 ns/op
BenchmarkLockFreeSlice-40         	 5000000	       281 ns/op
BenchmarkMultiLockFreeSlice-40    	10000000	       211 ns/op
BenchmarkSyncPool-40              	 1000000	      1138 ns/op
BenchmarkMultiSyncPool-40         	10000000	       197 ns/op
BenchmarkCASUnsafe-40             	100000000	        14.8 ns/op
BenchmarkCASInt64-40              	200000000	         8.04 ns/op
```