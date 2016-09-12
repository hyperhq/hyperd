# INTRODUCE

This module include the key functions on processing the POD JSON data file

The sample input POD data file is:

<pre><code>
{
	"id": "test_|-|_id",
	"containers" : [{
		"name": "web",
		"image": "nginx:latest"
	}],
	"resource": {
		"vcpu": 1,
		"memory": "128"
	},
	"files": [],
	"volumes": []
}
</code></pre>
