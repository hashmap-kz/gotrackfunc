### Example AST Injection

```
func (stream *StreamCtl) processOneMsg(ctx context.Context, msg pgproto3.BackendMessage) (*pglogrepl.CopyDoneResult, error) {
	defer func(start time.Time) {
		elapsed := time.Since(start)
		f, _ := os.OpenFile("gotrackfunc.log", os.O_APPEND|(os.O_CREATE|os.O_WRONLY), 0644)
		if f != nil {
			fmt.Fprintf(f, "%s 1 %d\n", "xlog.processOneMsg", elapsed.Nanoseconds())
			f.Close()
		}
	}(time.Now())
	
	.....
}
```


### Example Report

```
FUNCTION                            CALLS  TOTAL_NS     TOTAL_SEC
--------                            -----  --------     ---------
xlog.streamLog                      1      35698786991  35.70
xlog.ReceiveXlogStream              1      35690590769  35.69
httpsrv.Run                         1      35676578838  35.68
xlog.handleCopyStream               1      35674689601  35.67
xlog.processOneMsg                  12915  4178324954   4.18
xlog.processXLogDataMsg             12915  3684754155   3.68
fsync.Fsync                         308    2491208144   2.49
xlog.CloseWalFile                   100    2442316198   2.44
xlog.closeAndRename                 100    2433769275   2.43
fsync.FsyncFname                    201    1805949884   1.81
fsync.FsyncFnameAndDir              101    1256014201   1.26
xlog.WriteAtWalFile                 12915  657585406    0.66
fsync.FsyncDir                      102    627599302    0.63
xlog.SyncWalFile                    4      66217819     0.07
xlog.OpenWalFile                    101    31754052     0.03
xlog.CloseWalFileIfPresentNoRename  1      15692230     0.02
xlog.closeNoRename                  1      15572106     0.02
xlog.openFileAndFsync               1      11296206     0.01
xlog.createFileAndTruncate          100    5567943      0.01
xlog.updateLastFlushPosition        105    4602337      0.00
xlog.NewPgReceiver                  1      3283092      0.00
xlog.log                            39167  2793083      0.00
xlog.XLogFileName                   101    2552472      0.00
xlog.XLogSegmentOffset              25831  1704140      0.00
xlog.findStreamingStart             1      1689996      0.00
xlog.sendFeedback                   6      1457798      0.00
conv.ToUint64                       12967  1389159      0.00
metrics.AddWALBytesReceived         12915  1103206      0.00
conv.Uint64ToInt64                  13015  1081819      0.00
xlog.XLogFromFileName               51     381175       0.00
cmd.loadConfig                      1      310334       0.00
config.MustLoad                     1      258797       0.00
config.mustLoadCfg                  1      233486       0.00
xlog.IsXLogFileName                 52     207933       0.00
xlog.GetSlotInformation             2      167470       0.00
xlog.GetStartupInfo                 1      152233       0.00
xlog.parseReadReplicationSlot       2      116901       0.00
xlog.GetShowParameter               2      115980       0.00
httpsrv.InitHTTPHandlers            1      114010       0.00
xlog.parseShowParameter             2      68339        0.00
cmd.App                             1      63183        0.00
xlog.XLogSegmentsPerXLogId          253    52205        0.00
metrics.IncWALFilesReceived         100    49886        0.00
xlog.XLByteToSeg                    101    26878        0.00
xlog.strspnMap                      51     21621        0.00
xlog.ScanWalSegSize                 1      20857        0.00
cmd.needSupervisorLoop              1      19716        0.00
logger.Init                         1      19201        0.00
config.String                       1      13929        0.00
config.validate                     1      12211        0.00
xlog.calculateCopyStreamSleepTime   6      8553         0.00
xlog.IsPartialXLogFileName          52     6595         0.00
config.expand                       1      6349         0.00
jobq.Start                          1      5653         0.00
config.IsExternalStor               1      5132         0.00
optutils.HeredocTrim                1      4375         0.00
xlog.IsValidWalSegSize              1      4244         0.00
jobq.NewJobQueue                    1      3726         0.00
xlog.NewStream                      1      3232         0.00
conv.Uint64ToUint32                 51     1931         0.00
service.NewControlService           1      1425         0.00
httpsrv.NewHTTPSrv                  1      1318         0.00
cmd.checkPgEnvsAreSet               1      1209         0.00
httpsrv.log                         2      1124         0.00
middleware.Middleware               5      1002         0.00
config.expandEnvsWithPrefix         1      886          0.00
config.IsLocalStor                  2      498          0.00
conv.ParseUint32                    2      448          0.00
middleware.MiddlewareChain          2      376          0.00
middleware.SafeHandlerMiddleware    3      358          0.00
controller.NewController            1      338          0.00
xlog.existsTimeLineHistoryFile      1      306          0.00
xlog.SetStream                      1      257          0.00
config.Cfg                          2      233          0.00
xlog.IsPowerOf2                     1      165          0.00
conv.Uint32ToInt32                  1      113          0.00
xlog.XLogSegNoToRecPtr              1      108          0.00
```