### CORE (TODO)
We need a way to profile the resources(cpu and memory) that the application might take for a single request so that we can set the limit on the resources and scaling is also optimized based on the resource consumption.because in my DPA scaler this is the metric used and which would make the optimization to the predessesor HPABecause using value of cpu and memory utilization during the start of the pod(overhead) as cpu and mem usage per request is an incorrect  way to obtain the results
``` sh

OUTPUT:
-------------------------------------------------------
The deployment loadtestv2 has 1 pod/pods with 0.000000 avgCPU and 25.300781 avgMem utilization: 
The deployment has 500.000000 cpuLimit and 512.000000 memLimit set: 
max workload per pod (used memory since here memory is the bottleneck): 19 
predictedWorkload in rps 2 
currentWorkload in rps 1 
The current number of pods: 1
Scaling decision take to have: 0  of pods 
-----------------------------------------------------------
CONCLUSION: 
max workload per pod(which is in rps) should be much higher(like 10000000+) but here it is 19

```
### TEST
```go
package main

import "fmt"

func main(){
	currentWorkload:=1.0000000
	predictedWorkload:=2.000000
	cpuUtilization, memoryUtilization:= 1.000000,20.968750


	cpuLimit, memoryLimit:=  500.000000,512.000000

	fmt.Printf("The deployment has %f cpuLimit and %f memLimit set: \n", cpuLimit, memoryLimit)
	//compute the bottleneck resource
	var maxWorkloadPerPod float64
	var bottleneck string
	if (cpuLimit - cpuUtilization) < (memoryLimit - memoryUtilization) {
		bottleneck = "cpu"
		// Compute max workload per pod
		if cpuUtilization == 0 {
			fmt.Printf("%s utilization is 0, skipping scaling decision \n", bottleneck)
		}
		maxWorkloadPerPod = (cpuLimit * currentWorkload) / cpuUtilization
		fmt.Printf("max workload per pod (used cpu since here cpu is the bottleneck): %d rps\n", int(maxWorkloadPerPod))
		
	} else {
		bottleneck = "memory"
		// Compute max workload per pod
		if memoryUtilization == 0 {
			fmt.Printf("%s utilization is 0, skipping scaling decision \n", bottleneck)
		}
		maxWorkloadPerPod = (memoryLimit * currentWorkload) / memoryUtilization
		fmt.Printf("max workload per pod (used memory since here memory is the bottleneck): %d rps \n", int(maxWorkloadPerPod))

	}
	predictedFuturePods := int32(predictedWorkload / maxWorkloadPerPod)
	fmt.Printf("predictedFuturePods %d \n", predictedFuturePods)
	currentPods := 1

	fmt.Printf("The current number of pods: %d \n", currentPods)
	fmt.Printf("scaling decision take to have: %d  of pods\n", predictedFuturePods)
}
```
