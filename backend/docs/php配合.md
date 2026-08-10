<?php

//说明 efilter功能.1.判断ip是否和投放国家匹配 2.判断proxy是否为false 3.国家符合且非proxy，返回true，否则false 4.ip没返回国家的话，默认放行，返回true

$efilter_endpoint="http://127.0.0.1:8080/api/v1/results"
$efilter_api_key="risk-engine-dev-key-2026"
$real_page = 'index.html';//广告页面文件名
$safe_page = 'safe.html';//安全页文件名safe.html
$enable_efilter = true;//短路开关：true=走外部efilter接口判断；false=跳过外部请求直接放行真实页面
$filter_zh_language = true;//语言过滤：true=拦截中文浏览器语言(送安全页)；false=不过滤
$jsonData = array();
$jsonData['country']= 'US,AU,CA,GB,DE';//投放的国家地区，ALL为全可以访问，留空为全部不允许


// ============================
// 1. 只有 go=1 才验证
// ============================

if (isset($_GET['go']) && $_GET['go'] == '1') {

    $secret = "eflp";

    $t = intval($_GET['t'] ?? 0);
    $n = $_GET['n'] ?? '';
    $s = $_GET['s'] ?? '';

    $now = floor(time() / 10);

    $ok =
        ($t == $now - 1 && $s === substr(md5($secret . ($now - 1) . $n), 0, 10)) ||
        ($t == $now     && $s === substr(md5($secret . $now . $n), 0, 10)) ||
        ($t == $now + 1 && $s === substr(md5($secret . ($now + 1) . $n), 0, 10));

    if ($ok) {
        echo file_get_contents($real_page);
        exit;
    }

    echo file_get_contents($safe_page);
    exit;
}

// ============================
// 2. lptoken验证
// ============================

$key = "51d120a77c5c31d39089eee5a5736d84";
$token = @$_GET["lptoken"];
if(empty($token)) exit(0);
if(strlen($token) !== 20) exit(0);
$ts = substr($token, 0, 2) . substr($token, 4, 2) . substr($token, 8, 2) . substr($token, 12, 2) . substr($token, 16, 2);
$h = substr($token, 2, 2) . substr($token, 6, 2) . substr($token, 10, 2) . substr($token, 14, 2) . substr($token, 18, 2);
$m = md5($key . $ts . $_SERVER["HTTP_USER_AGENT"]);
$mp = substr($m, 0, 2) . substr($m, 5, 2) . substr($m, 12, 2) . substr($m, 19, 2) . substr($m, 26, 2);
if (time() > $ts || $mp !== $h) {
  exit(0);
}


// IP获取逻辑保持不变
if(isset($_SERVER['HTTP_X_SHOPIFY_CLIENT_IP'])){
    $ip = $_SERVER['HTTP_X_SHOPIFY_CLIENT_IP']; 
}
else if (isset($_SERVER['HTTP_CF_CONNECTING_IP'])) {
    $ip = $_SERVER['HTTP_CF_CONNECTING_IP'];
} else {
    if (getenv('HTTP_CLIENT_IP') && strcasecmp(getenv('HTTP_CLIENT_IP'), 'unknown')) {
        $ip = getenv('HTTP_CLIENT_IP');
    } elseif (getenv('HTTP_X_FORWARDED_FOR') && strcasecmp(getenv('HTTP_X_FORWARDED_FOR'), 'unknown')) {
        $ip = getenv('HTTP_X_FORWARDED_FOR');
    } elseif (getenv('REMOTE_ADDR') && strcasecmp(getenv('REMOTE_ADDR'), 'unknown')) {
        $ip = getenv('REMOTE_ADDR');
    } elseif (isset($_SERVER['REMOTE_ADDR']) && $_SERVER['REMOTE_ADDR'] && strcasecmp($_SERVER['REMOTE_ADDR'], 'unknown')) {
        $ip = $_SERVER['REMOTE_ADDR'];
    }
}
$ip_arr=explode(',',$ip);
$jsonData['ip']=$ip_arr[0];

// ============================
// 3.浏览器语言过滤（在 curl 之前本地短路）
// ============================

function detectLanguage() {
    if (isset($_SERVER['HTTP_ACCEPT_LANGUAGE'])) {
        $acceptLang = strtolower($_SERVER['HTTP_ACCEPT_LANGUAGE']);
        // 检测各种中文语言代码：zh, zh-cn, zh-tw, zh-hk, zh-sg 等
        if (preg_match('/zh(-[a-z]{2})?/', $acceptLang)) {
            return 'zh';
        }
        return 'non-zh';
    }
    return 'unknown';
}

$language = detectLanguage();

if ($filter_zh_language && $language == 'zh') {
    echo file_get_contents($safe_page);
    exit;
}

// ============================
// 4. efilter 接口验证
// ============================

if (!$enable_efilter) {
    //短路开关关闭：不走外部 efilter 接口，默认放行真实页面
    $boolean = true;
} elseif (in_array($jsonData['ip'],$white_ip)){
    $boolean=true;
}else{
    $ch = curl_init($efilter_endpoint);
    curl_setopt($ch, CURLOPT_SSL_VERIFYPEER, false);
    curl_setopt($ch, CURLOPT_SSL_VERIFYHOST, false);  
    <!-- curl_setopt($ch, CURLOPT_USERPWD, "$username:$password"); -->
    <!-- 这个userpwd换成key -->
    curl_setopt($ch, CURLOPT_TIMEOUT, 3); // 增加超时时间到3秒 2
    curl_setopt($ch, CURLOPT_CONNECTTIMEOUT, 2); // 连接超时2秒 1
    curl_setopt($ch, CURLOPT_POST, 1);
    curl_setopt($ch, CURLOPT_POSTFIELDS, http_build_query($jsonData));
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
    
    $return = curl_exec($ch);
    $http_code = curl_getinfo($ch, CURLINFO_HTTP_CODE);
    
    // 容错处理：如果请求失败或服务器无响应，默认显示真实页面
    if ($return === false || curl_error($ch) || $http_code !== 200) {
        $boolean = true; // 默认允许访问真实页面
    } else {
        $return = json_decode($return, true);
        $boolean = isset($return['result']) ? $return['result'] : true; // 如果解析失败也默认允许
    }
    curl_close($ch);
}




// ============================
// 5. 正常逻辑执行
// ============================

if ($boolean){
    echo file_get_contents($real_page);
}else{
    echo file_get_contents($safe_page);
}
?>