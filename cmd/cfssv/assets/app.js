$(document).ready( function(){

	var appRoot = '/ui';
	var apiRoot = '/api';
	var routes = [];

	function init(){
		// pushState を使った遷移
		$(document).on('click', 'a', function(e) {
			if( $(this).hasClass('noajax') ){
				return;
			}
			var url = $(this).attr('href');
			if( url.startsWith('/') ){
				e.preventDefault();
				jump(url);
			}
		});

		$(window).on('popstate', function(e){
			jump(location.pathname);
		});

		$('[data-toggle="tooltip"]').tooltip();
		
		jump(window.location.pathname);
	}

	function jump(url){
		var suburl = url.replace(appRoot,'');
		//console.log('jump to ' + url);
		for( var i in routes ){
			var r = routes[i];
			var matches = suburl.match( r[0] );
			if( matches ){
				var params = {};
				for( var j = 0; j < r[1].length; j++ ){
					params[r[1][j]] = matches[j+1];
				}
				r[2](params);
				history.pushState(null,null,url);
				return;
			}
		}
		console.log('invalid url ' + url);
	}

	function nav(id){
		$('.navbar-nav li').removeClass('active');
		$('.navbar-nav li'+id).addClass('active');
	}

	function render(templateId, param){
		param = param || {};
		var template = $('#'+templateId).text();
		if( !template ){
			throw 'invalid template "'+templateId+'"';
		}
		$('.content').html(TemplateEngine(template, param));
	}

	function route(path, func){
		var names = [];
		var regexpStr = path.replace(/:(\w+)/, function(name){
			names.push(name.substr(1));
			return '([^/]+)';
		});
		var regexp = new RegExp(regexpStr+'$');
		// console.log(regexpStr, regexp);
		routes.push([regexp, names, func]);
	}

	// micro template engine
	// From http://krasimirtsonev.com/blog/article/Javascript-template-engine-in-just-20-line
	var TemplateEngine = function(html, options) {
		var re = /<%([^%>]+)?%>/g, reExp = /(^( )?(if|for|else|switch|case|break|{|}))(.*)?/g, code = 'var r=[];\n', cursor = 0, match;
		var add = function(line, js) {
			js? (code += line.match(reExp) ? line + '\n' : 'r.push(' + line + ');\n') :
            (code += line != '' ? 'r.push("' + line.replace(/"/g, '\\"') + '");\n' : '');
			return add;
		};
		while(true){
			match = re.exec(html);
			if( !match) break;
			add(html.slice(cursor, match.index))(match[1], true);
			cursor = match.index + match[0].length;
		}
		add(html.substr(cursor, html.length - cursor));
		code += 'return r.join("");';
		return new Function(code.replace(/[\r\t\n]/g, '')).apply(options);
	};

	route('/', function(){
		nav('#nav-index');
		$.getJSON(apiRoot+'/tags',function(data){
			render('index', {tags: data});
		});
	});

	route('/stat', function(){
		nav('#nav-stat');
		$.getJSON(apiRoot+'/stat',function(data){
			render('stat', data);
		});
	});

	route('/tags/:id', function(params){
		$.getJSON(apiRoot+'/tags/'+params.id, function(data){
			render('tags-index', data);
		});
	});

	route('/tags/:id/files/:file', function(params){
		$.get(apiRoot+'/tags/'+params.id+'/'+params.file, function(data){
			render('tags-files', data);
		});
	});

	route('/tags/:id/versions', function(params){
		$.get(apiRoot+'/tags/'+params.id+'/versions', function(data){
			render('tags-versions-index', {id: params.id, versions: data});
		});
	});
	
	route('/tags/:id/versions/:version', function(params){
		$.get(apiRoot+'/tags/'+params.id+'/versions/'+params.version, function(data){
			render('tags-files', data);
		});
	});
	
	init();
});
