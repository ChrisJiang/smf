package consumer

import (
    "context"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/free5gc/openapi"
    "github.com/free5gc/openapi/models"
    smf_context "github.com/free5gc/smf/internal/context"
    "github.com/free5gc/smf/internal/logger"
)

func SendNFRegistration() (models.NfProfile, error) {
    smfProfile := smf_context.NFProfile

    sNssais := []models.Snssai{}
    for _, snssaiSmfInfo := range *smfProfile.SMFInfo.SNssaiSmfInfoList {
        sNssais = append(sNssais, *snssaiSmfInfo.SNssai)
    }

    // set nfProfile
    profile := models.NfProfile{
        NfInstanceId:  smf_context.GetSelf().NfInstanceID,
        NfType:        models.NfType_SMF,
        NfStatus:      models.NfStatus_REGISTERED,
        Ipv4Addresses: []string{smf_context.GetSelf().RegisterIPv4},
        NfServices:    smfProfile.NFServices,
        SmfInfo:       smfProfile.SMFInfo,
        SNssais:       &sNssais,
        PlmnList:      smfProfile.PLMNList,
    }

    if smf_context.GetSelf().Locality != "" {
        profile.Locality = smf_context.GetSelf().Locality
    }
    var pf models.NfProfile
    var res *http.Response
    var err error

    // Check data (Use RESTful PUT)
    for {
        pf, res, err = smf_context.GetSelf().
            NFManagementClient.
            NFInstanceIDDocumentApi.
            RegisterNFInstance(context.TODO(), smf_context.GetSelf().NfInstanceID, profile)
        if err != nil || res == nil {
            logger.ConsumerLog.Infof("SMF register to NRF Error[%s]", err.Error())
            time.Sleep(2 * time.Second)
            continue
        }
        defer func() {
            if resCloseErr := res.Body.Close(); resCloseErr != nil {
                logger.ConsumerLog.Errorf("RegisterNFInstance response body cannot close: %+v", resCloseErr)
            }
        }()

        status := res.StatusCode
        if status == http.StatusOK {
            // NFUpdate
            break
        } else if status == http.StatusCreated {
            // NFRegister
            resourceUri := res.Header.Get("Location")
            // resouceNrfUri := resourceUri[strings.LastIndex(resourceUri, "/"):]
            smf_context.GetSelf().NfInstanceID = resourceUri[strings.LastIndex(resourceUri, "/")+1:]

            oauth2 := false
            if pf.CustomInfo != nil {
                v, ok := pf.CustomInfo["oauth2"].(bool)
                if ok {
                    oauth2 = v
                    logger.MainLog.Infoln("OAuth2 setting receive from NRF:", oauth2)
                }
            }
            smf_context.GetSelf().OAuth2Required = oauth2
            if oauth2 && smf_context.GetSelf().NrfCertPem == "" {
                logger.CfgLog.Error("OAuth2 enable but no nrfCertPem provided in config.")
            }
            break
        } else {
            logger.ConsumerLog.Infof("handler returned wrong status code %d", status)
            // fmt.Errorf("NRF return wrong status code %d", status)
        }
    }

    logger.InitLog.Infof("SMF Registration to NRF %v", pf)
    return pf, nil
}

func RetrySendNFRegistration(MaxRetry int) (models.NfProfile, error) {
    retryCount := 0
    var pf models.NfProfile
    var err error
    for retryCount < MaxRetry {
        pf, err = SendNFRegistration()
        if err == nil {
            return pf, err
        }
        logger.ConsumerLog.Warnf("Send NFRegistration Failed by %v", err)
        retryCount++
    }

    return pf, fmt.Errorf("[SMF] Retry NF Registration has meet maximum")
}

func SendNFDeregistration() error {
    // Check data (Use RESTful DELETE)

    ctx, _, err := smf_context.GetSelf().GetTokenCtx("nnrf-nfm", "NRF")
    if err != nil {
        return err
    }

    res, localErr := smf_context.GetSelf().
        NFManagementClient.
        NFInstanceIDDocumentApi.
        DeregisterNFInstance(ctx, smf_context.GetSelf().NfInstanceID)
    if localErr != nil {
        logger.ConsumerLog.Warnln(localErr)
        return localErr
    }
    defer func() {
        if resCloseErr := res.Body.Close(); resCloseErr != nil {
            logger.ConsumerLog.Errorf("DeregisterNFInstance response body cannot close: %+v", resCloseErr)
        }
    }()
    if res != nil {
        if status := res.StatusCode; status != http.StatusNoContent {
            logger.ConsumerLog.Warnln("handler returned wrong status code ", status)
            return openapi.ReportError("handler returned wrong status code %d", status)
        }
    }
    return nil
}

func SendDeregisterNFInstance() (*models.ProblemDetails, error) {
    logger.ConsumerLog.Infof("Send Deregister NFInstance")

    ctx, pd, err := smf_context.GetSelf().GetTokenCtx("nnrf-nfm", "NRF")
    if err != nil {
        return pd, err
    }

    smfSelf := smf_context.GetSelf()
    // Set client and set url

    res, err := smfSelf.
        NFManagementClient.
        NFInstanceIDDocumentApi.
        DeregisterNFInstance(ctx, smfSelf.NfInstanceID)
    if err == nil {
        return nil, err
    } else if res != nil {
        defer func() {
            if resCloseErr := res.Body.Close(); resCloseErr != nil {
                logger.ConsumerLog.Errorf("DeregisterNFInstance response body cannot close: %+v", resCloseErr)
            }
        }()
        if res.Status != err.Error() {
            return nil, err
        }
        problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
        return &problem, err
    } else {
        return nil, openapi.ReportError("server no response")
    }
}
type StringType string
// CreateSubscription
func CreateNfSubscription(subscrCond StringType) (models.NrfSubscriptionData, error) {
    smfProfile := smf_context.NFProfile
	plmnList := *smfProfile.PLMNList

    // set NrfSubscriptionData
    sub := models.NrfSubscriptionData{
        NfStatusNotificationUri:  fmt.Sprintf("%s://%s:%d/nnrf-nfm/v1/nf-status-notify",
                                        smf_context.GetSelf().URIScheme,
                                        smf_context.GetSelf().RegisterIPv4,
                                        smf_context.GetSelf().SBIPort),
        SubscrCond:               subscrCond,
        PlmnId:                   &plmnList[0],
        ReqNfType:                models.NfType_SMF,
        //SubscriptionId
        //ValidityTime
        //ReqNotifEvents
        //NotifCondition
        //ReqNfFqdn
    }

    var subRet models.NrfSubscriptionData
    var res *http.Response
    var err error

    // Check data (Use RESTful PUT)
    for {
        subRet, res, err = smf_context.GetSelf().
            NFManagementClient.
            SubscriptionsCollectionApi.
            CreateSubscription(context.TODO(), sub)
        if err != nil || res == nil {
            logger.ConsumerLog.Infof("SMF subscription creation to NRF Error[%s]", err.Error())
            time.Sleep(2 * time.Second)
            continue
        }
        defer func() {
            if resCloseErr := res.Body.Close(); resCloseErr != nil {
                logger.ConsumerLog.Errorf("CreateSubscription response body cannot close: %+v", resCloseErr)
            }
        }()

        status := res.StatusCode
        if status == http.StatusOK {
            // ??
            break
        } else if status == http.StatusCreated {
            // CreateSubscription
            resourceUri := res.Header.Get("Location")
            subscriptionId := resourceUri[strings.LastIndex(resourceUri, "/")+1:]
			logger.InitLog.Infof("SMF subscription id %s for subscrCond %s", subscriptionId, subscrCond)
            break
        } else {
            logger.ConsumerLog.Infof("handler returned wrong status code %d", status)
            // fmt.Errorf("NRF return wrong status code %d", status)
        }
    }

    logger.InitLog.Infof("SMF subscription creation to NRF %v", subRet)
    return subRet, nil
}