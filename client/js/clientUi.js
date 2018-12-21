let nodesTimer = null
let messagesTimer = null
let filesTimer = null

$(document).ready(function(){
    
    // Get the name of the peer
    getName()
    getNodes()
    getMessages()
    getFileList()

    nodesTimer = setInterval(getNodes, 1000)
    messagesTimer = setInterval(getMessages, 1000)
    filesTimer = setInterval(getFileList, 100000)
    
	$.fn.exists = function() {
		return this.length !== 0
	}
    
    // Send Message button
	$("#sendMessage").click(function() {
		const msg = $("#message").val()
		$("#sendMessage").prop("disabled", true)
        $("#message").prop("disabled", true)
        var obj = { Dest: "", Msg: msg, File: "", Request: "", Keywords: "", Budget: 0 }
        var dataToSend = JSON.stringify(obj)
        $.ajax({
            type: 'POST',
            url: "/message",
            data: dataToSend,
            success: function() {
                getMessages()
                $("#sendMessage").prop("disabled", false)
                $("#message").prop("disabled", false)
                $("#message").val("")
            },
            error: function() {
                alert("Unable to send message")
                $("#sendMessage").prop("disabled", false)
                $("#message").prop("disabled", false)
            },
            contentType: "application/json"
        })
    })

    // Send Peer button
    $("#sendPeer").click(function(){
        const peer = $("#peerAddress").val()
        $.ajax({
            type: 'POST',
            url: "/node",
            data: JSON.stringify(peer),
            success: function() {
                getNodes()
                $("#peerAddress").val("")
            },
            error: function() {
                alert("Unable to add peer")
                $("#peerAddress").val("")
            },
            contentType: "application/json"
        })
    })

    // Download button
    $("#downloadButton").click(function(){
        const dest = $("#downloadNode").val()
        const file = $("#fileName").val()
        const request = $("#metaHash").val()
        if (dest !== "" && file !== "" && request !== "") {
            var obj = { Dest: dest, Msg: "", File: file, Request: request, Keywords: "", Budget: 0 }
            var dataToSend = JSON.stringify(obj)
            $.ajax({
                type: 'POST',
                url: "/message",
                data: dataToSend,
                success: function() {
                    alert("File downloaded")
                },
                error: function() {
                    alert("Unable to download file")
                },
                contentType: "application/json"
            })
        } else {
            alert("Please insert all the information")
        }
    })

    // Select file button
    $("#shareFile").click(function(){
        const selectFile = window.document.getElementById("selectFile")
        var file = selectFile[selectFile.selectedIndex].value
        //alert(file)
        var obj = { Dest: "", Msg: "", File: file, Request: "", Keywords: "", Budget: 0 }
        var dataToSend = JSON.stringify(obj)
        $.ajax({
            type: 'POST',
            url: "/message",
            data: dataToSend,
            success: function() {
                alert("File shared")
            },
            error: function() {
                alert("Unable to share file")
            },
            contentType: "application/json"
        })
    })

    // Search file button
    $("#searchButton").click(function(){
        const searchFile = $("#searchFile").val()
        var budget = parseFloat($("#budget").val())
        if (isNaN(budget)) {
            budget = 0
        }
        var obj = { Dest: "", Msg: "", File: "", Request: "", Keywords: searchFile, Budget: budget }
        var dataToSend = JSON.stringify(obj)
        $.ajax({
            type: 'POST',
            url: "/message",
            data: dataToSend,
            //success: function() {
            //    var search = setInterval(getSearchedFiles, 500);
            //    setTimeout(clearInterval(search), 5000)
            //},
            error: function() {
                alert("Unable to search keywords")
            },
            contentType: "application/json"
        })
        // Reset results
        resetResultSearchFiles()
        // Ping if there are succesful files
        var search = setInterval(function() {getSearchedFileNames(searchFile)}, 500);
        setTimeout(function() { clearInterval( search ); }, 20000);
    })
})

function getName() {
    $.get("/id", function(data){
        const name = JSON.parse(data)
        $(".nodeName").text(name)
    })
}

function getFileList() {
    $.get("/message?file=all", function(files){
        //alert(files)
        var selectFile = window.document.getElementById("selectFile")
        //var i;
        //for (i = 1; i <= selectFile.length-1; i++) { 
        //    selectFile.remove(i);
        //}

        if (selectFile !== null) {
            JSON.parse(files)
            //.sort((x, y) => x.localeCompare(y))
            .forEach(file => {
                if (selectFile.namedItem(file) === null) {
                    const elem = document.createElement("option")
                    elem.id = file
                    elem.appendChild(document.createTextNode(file))
                    selectFile.appendChild(elem)
                }
            })
        }
    })
}

function getSearchedFileNames(keywords) {
    //alert(keywords)
    $.get("/message?keywords="+keywords, function(files){
        //alert(files)
        const resultContent = window.document.getElementById("resultContent")

        if (resultContent !== null && files !== null) {
            //alert("Inside")
            JSON.parse(files)
            .forEach(f => {
                //<a onClick="a();" style="cursor: pointer; cursor: hand;">*click here*</a>
                //object.onclick = function(){myScript};
                const elem = document.createElement("div")
                elem.setAttribute("style", "cursor: pointer; cursor: hand;")
                elem.onclick = function(){downloadSearchedFile(f)}
				elem.appendChild(document.createTextNode(f))
				resultContent.appendChild(elem)
			})
        }
    })
}

function resetResultSearchFiles() {
    const resultContent = window.document.getElementById("resultContent")
    if (resultContent !== null) {
        resultContent.innerHTML = "Results:"
    }
}

function downloadSearchedFile(file) {
    //alert(file)
    $.get("/message?file="+file, function(message){
        alert(message)
    })
}

function getNodes() {
    $.get("/node", function(nodes){

        const peerBox = document.getElementById("peerContent")
        const nodeBox = document.getElementById("nodeContent")

        var parsed = JSON.parse(nodes)
        //alert(parsed.Peers)

        if (peerBox !== null) {
			peerBox.innerHTML = "Peers"
            parsed.Peers
            .sort((x, y) => x.localeCompare(y))
            .forEach(n => {
                const elem = document.createElement("div")
				elem.appendChild(document.createTextNode(n))
				peerBox.appendChild(elem)
			})
        }

        
        if (nodeBox !== null) {
            nodeBox.innerHTML = "Nodes"
            if (parsed.Nodes !== null) {
                parsed.Nodes
                .sort((x, y) => x.localeCompare(y))
                .forEach(dest => {
                    const elem = document.createElement("div")
                    const buttonElem = document.createElement("button")
                    buttonElem.addEventListener('click', function(){
                        openPrivateDialogBox(dest);
                    });
                    buttonElem.appendChild(document.createTextNode(dest))
                    elem.appendChild(buttonElem)
                
                    nodeBox.appendChild(elem)
                })
            }
        }
    })
}

function openPrivateDialogBox(dest) {

    //alert(dest)
    var script = `
    <head>  
    <meta charset="utf-8" /> 
    <link rel="stylesheet" type="text/css" href="css/style.css" /> 
    <script type="text/javascript" src="js/jquery-3.2.1.js"></script> 
    <script type="text/javascript" src="js/jquery-ui.js"></script> 
</head>
<body> 
    <div id="applicationBox"> 
        <div class="title" id="destination">` + dest + `</div> 
        <div class="clear" id="inputBox"> 
            <div class="border right"> 
                Message: <input type="text" placeholder="Write a private message" id="privateMessage" /> <button id="sendPrivateMessage">Send</button><br /> 
            </div> 
        </div> 
        <div id="privateChatBox"> 
            <div class="border left" id="privateChatContent"> 
                Private Chat 
            </div> 
        </div> 
    </div> 
</body>
<script>

    var dest = window.document.getElementById("destination").textContent
    //alert(dest)

    function getPrivateMessages() {
        $.get("/message?dest=" + dest, function(messages){
            const privateChatBox = window.document.getElementById("privateChatContent")
    
            if (privateChatBox !== null) {
                privateChatBox.innerHTML = "Private Chat"
                JSON.parse(messages)
                //.sort((x, y) => x.localeCompare(y))
                .forEach(m => {
                    const elem = document.createElement("div")
                    elem.appendChild(document.createTextNode(m.Origin + ": " + m.Text))
                    privateChatBox.appendChild(elem)
                })
            }
        })
    }

    getPrivateMessages()
    privateMessagesTimer = setInterval(getPrivateMessages, 1000)
    
    $.fn.exists = function() {
        return this.length !== 0
    }
    
    // Send Message button
    $("#sendPrivateMessage").click(function() {
        //alert("PROVA")
        const msg = $("#privateMessage").val()
        $("#sendPrivateMessage").prop("disabled", true)
        $("#privateMessage").prop("disabled", true)
        var obj = { Dest: dest, Msg: msg, File: "", Request: "", Keywords: "", Budget: 0 }
        var dataToSend = JSON.stringify(obj)
        $.ajax({
            type: 'POST',
            url: "/message",
            data: dataToSend,
            success: function() {
                getPrivateMessages()
                $("#sendPrivateMessage").prop("disabled", false)
                $("#privateMessage").prop("disabled", false)
                $("#privateMessage").val("")
            },
            error: function() {
                alert("Unable to send message")
                $("#sendPrivateMessage").prop("disabled", false)
                $("#privateMessage").prop("disabled", false)
            },
            contentType: "application/json"
        })
    })
</script> 
     `
    var win = window.open('','popUpWindow','height=400,width=600,left=10,top=10,,scrollbars=yes,menubar=no') 
    win.document.write(script);
    win.destination = dest

    return false;
}



function getMessages() {
    $.get("/message", function(messages){

        const chatBox = document.getElementById("chatContent")

        if (chatBox !== null) {
			//chatBox.innerHTML = "Chat"
            JSON.parse(messages)
            //.sort((x, y) => x.localeCompare(y))
            .forEach(n => {
                const elem = document.createElement("div")
				elem.appendChild(document.createTextNode(n.Origin + ": " + n.Text))
				chatBox.appendChild(elem)
			})
		}
    })
}
