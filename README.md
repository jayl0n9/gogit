# gogit
golang实现的githack,自动解析源码文件并保存，为https://github.com/lijiejie/GitHack 的golang版本
由于lijiejie大佬的githack制作解析下载，有时解析出来的文件无法下载，于是加上了-down功能，下载完整的.git泄露
其次加入了许多

~~~
使用方法：gogit -u http://example.com/.git


-uf filepath 批量扫描并保存
-down       爬取.git目录，并下载完整的.git泄露的文件（后续由自己手动使用git命令对泄露进行解析）
-all        不仅爬取.git，还会再使用lijiejie的githack的方式进行解析下载一遍，并且还会默认带上-o的功能

以下参数为筛选功能，筛选掉不需要的进行下载，比如一下pdf，word，都可以进行指定，多个后缀、名字用逗号隔开
-e string  不保存指定后缀的文件
-i string  只保存指定后缀的文件
-en string 不保存名字中带某字符串的文件
-n string 只保存名字中带指定字符串的文件
-o        是否保存解析出的所有文件的文件路径（可用于未授权扫描字典，文件保存在生成的目录中的gitAllUrl.txt中）

~~~
