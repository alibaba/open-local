package controller

import (
	"context"
	"regexp"
	"time"

	localtype "github.com/alibaba/open-local/pkg"
	"github.com/alibaba/open-local/pkg/restic"
	"github.com/alibaba/open-local/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

// 背景: restic 不会自己删除 repo, 即使 repo 中没有快照
// https://github.com/restic/restic/issues/1977
// There's no way to let restic delete a repository completely. I think having a way to delete an entire repo is not so important...
func (c *Controller) CleanUnusedResticRepo() {
	c.mux.Lock()
	defer c.mux.Unlock()
	klog.Infof("CleanUnusedResticRepo: start clean unused restic repo...")
	defer utils.TimeTrack(time.Now(), "clean s3 unused restic repo")

	var (
		expForYoda  = regexp.MustCompile(restic.ClusterID + "/yoda-[a-z0-9-]+")
		expForLocal = regexp.MustCompile(restic.ClusterID + "/local-[a-z0-9-]+")
	)

	snapshotContents, err := c.snapshotContentLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("CleanUnusedResticRepo: fail to list snapshotContents: %s", err.Error())
		return
	}

	// 获取快照对应的 pv-id map
	// key: class name
	// value: content map
	sourceIdMap := map[string]struct{}{}
	for _, content := range snapshotContents {
		if !utils.ContainsProvisioner(content.Spec.Driver) {
			continue
		}
		if content.Spec.Source.VolumeHandle == nil {
			continue
		}
		sourceIdMap[*content.Spec.Source.VolumeHandle] = struct{}{}
	}

	// controller 定期 list provisioner 的 snapshot class
	snapshotClasses, err := c.snapshotClassLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("CleanUnusedResticRepo: fail to list snapshotClasses: %s", err.Error())
		return
	}

	for _, class := range snapshotClasses {
		if !utils.ContainsProvisioner(class.Driver) {
			continue
		}

		// 通过 param 来判断有读写快照
		secretName, exist := class.Parameters[localtype.ParamSnapshotSecretName]
		if !exist {
			klog.Warningf("CleanUnusedResticRepo: no param %s found in volumesnapshotclass %s, skip...", localtype.ParamSnapshotSecretName, class.Name)
			continue
		}
		secretNamespace, exist := class.Parameters[localtype.ParamSnapshotSecretNamespace]
		if !exist {
			klog.Warningf("CleanUnusedResticRepo: no param %s found in volumesnapshotclass %s, skip...", localtype.ParamSnapshotSecretNamespace, class.Name)
			continue
		}
		secret, err := c.kubeclientset.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("CleanUnusedResticRepo: fail to get secret %s/%s, skip...: %s", secretNamespace, secretName, err.Error())
			continue
		}
		// 获取到 s3 secret 信息，通过 s3 sdk list 有哪些 repo
		s3URL := string(secret.Data[localtype.S3_URL])
		s3AK := string(secret.Data[localtype.S3_AK])
		s3SK := string(secret.Data[localtype.S3_SK])
		s3REGION := string(secret.Data[localtype.S3_Region])
		s3Token := string(secret.Data[localtype.S3_Token])
		if s3REGION == "" {
			s3REGION = "china"
		}
		s3DisableSSL := string(secret.Data[localtype.S3_DisableSSL])
		s3ForcePathStyle := string(secret.Data[localtype.S3_ForcePathStyle])

		cred := credentials.NewStaticCredentials(s3AK, s3SK, s3Token)
		config := aws.Config{
			Region:           aws.String(s3REGION),
			Endpoint:         aws.String(s3URL),
			DisableSSL:       aws.Bool(s3DisableSSL == "true"),
			S3ForcePathStyle: aws.Bool(s3ForcePathStyle == "true"),
			Credentials:      cred,
		}
		// Create S3 service client
		s, err := session.NewSession(&config)
		if err != nil {
			klog.Errorf("CleanUnusedResticRepo: fail to create s3 session: %s", err.Error())
			continue
		}
		s3Client := s3.New(s)
		bucketName := restic.RepoBucket
		_, err = s3Client.HeadBucket(&s3.HeadBucketInput{
			Bucket: &bucketName,
		})
		if err != nil {
			klog.Errorf("CleanUnusedResticRepo: fail to head bucket: %s", err.Error())
			continue
		}

		prefix := restic.ClusterID
		req := &s3.ListObjectsV2Input{
			Bucket: &bucketName,
			Prefix: &prefix,
		}

		var ret []string
		err = s3Client.ListObjectsV2Pages(req, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
			for _, obj := range page.Contents {
				ret = append(ret, *obj.Key)
			}
			return !lastPage
		})
		if err != nil {
			klog.Errorf("CleanUnusedResticRepo: fail to list objects: %s", err.Error())
			continue
		}
		prefixNames := map[string][]string{}
		for _, objectName := range ret {
			// objectName: [clusterid]/yoda-f2274fa6-b494-4de8-afcd-76dcb3ccd95c/data/ae/aed52a251cca3cf139b2759fb17f5553d2463426bd67d109767f36fde98ac47c
			// prefixName: [clusterid]/yoda-f2274fa6-b494-4de8-afcd-76dcb3ccd95c
			if prefixName := expForYoda.FindString(objectName); prefixName != "" {
				objects := prefixNames[prefixName]
				if objects == nil {
					objects = []string{}
				}
				objects = append(objects, objectName)
				prefixNames[prefixName] = objects
			}
			if prefixName := expForLocal.FindString(objectName); prefixName != "" {
				objects := prefixNames[prefixName]
				if objects == nil {
					objects = []string{}
				}
				objects = append(objects, objectName)
				prefixNames[prefixName] = objects
			}
		}

		// 逐个判断是否需要删除 restic repository
		// 根据环境中是否有相关 snapshotcontent 的 source id 为 pv id
		for prefixName, objectNames := range prefixNames {
			_, exist := sourceIdMap[prefixName[len(restic.ClusterID)+1:]]
			if exist {
				// 表示此 repo 有对应的 snapshot 资源指向
				klog.Infof("CleanUnusedResticRepo: repo %s is protected, skip...", prefixName)
				continue
			}
			klog.Infof("CleanUnusedResticRepo: start to remove repo %s...", prefixName)
			for _, objectName := range objectNames {
				_, err = s3Client.DeleteObject(&s3.DeleteObjectInput{
					Bucket: &bucketName,
					Key:    &objectName,
				})
				if err != nil {
					klog.Errorf("CleanUnusedResticRepo: fail to remove object %s: %s", prefixName, err.Error())
					continue
				}
			}
		}
	}
}
