/**
 * 坎儿井三维剖面可视化
 * 使用Three.js实现透明管道暗渠 + 粒子水流动画
 */

class Karez3DViewer {
    constructor(containerId) {
        this.container = document.getElementById(containerId);
        this.scene = null;
        this.camera = null;
        this.renderer = null;
        this.controls = null;
        this.clock = new THREE.Clock();
        
        this.segments = [];
        this.shafts = [];
        this.particleSystems = [];
        this.shaftMeshes = [];
        this.aqueductMeshes = [];
        this.groundMesh = null;
        
        this.showParticles = true;
        this.showShafts = true;
        this.simSpeed = 1.0;
        
        this.flowData = {};
        
        this.init();
    }

    init() {
        const width = this.container.clientWidth;
        const height = this.container.clientHeight;

        this.scene = new THREE.Scene();
        this.scene.background = new THREE.Color(0x0d1b2a);
        this.scene.fog = new THREE.Fog(0x0d1b2a, 100, 500);

        this.camera = new THREE.PerspectiveCamera(60, width / height, 0.1, 2000);
        this.camera.position.set(60, 40, 80);

        this.renderer = new THREE.WebGLRenderer({ 
            antialias: true,
            alpha: true 
        });
        this.renderer.setSize(width, height);
        this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
        this.renderer.shadowMap.enabled = true;
        this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
        
        const canvas = this.container.querySelector('canvas');
        if (canvas) {
            this.renderer.domElement = canvas;
        } else {
            this.container.appendChild(this.renderer.domElement);
        }

        this.controls = new THREE.OrbitControls(this.camera, this.renderer.domElement);
        this.controls.enableDamping = true;
        this.controls.dampingFactor = 0.05;
        this.controls.minDistance = 10;
        this.controls.maxDistance = 300;
        this.controls.maxPolarAngle = Math.PI / 2 + 0.1;

        this.setupLights();
        this.createGround();
        this.createDefaultKarez();

        window.addEventListener('resize', () => this.onResize());

        this.animate();
    }

    setupLights() {
        const ambientLight = new THREE.AmbientLight(0x404060, 0.6);
        this.scene.add(ambientLight);

        const sunLight = new THREE.DirectionalLight(0xfff0e0, 0.8);
        sunLight.position.set(50, 100, 30);
        sunLight.castShadow = true;
        sunLight.shadow.mapSize.width = 2048;
        sunLight.shadow.mapSize.height = 2048;
        sunLight.shadow.camera.near = 0.5;
        sunLight.shadow.camera.far = 500;
        sunLight.shadow.camera.left = -100;
        sunLight.shadow.camera.right = 100;
        sunLight.shadow.camera.top = 100;
        sunLight.shadow.camera.bottom = -100;
        this.scene.add(sunLight);

        const fillLight = new THREE.DirectionalLight(0x6080ff, 0.3);
        fillLight.position.set(-30, 20, -50);
        this.scene.add(fillLight);

        const waterGlow = new THREE.PointLight(0x4fc3f7, 0.5, 100);
        waterGlow.position.set(0, -10, 0);
        this.scene.add(waterGlow);
    }

    createGround() {
        const groundGeometry = new THREE.PlaneGeometry(300, 200, 50, 50);
        
        const positions = groundGeometry.attributes.position;
        for (let i = 0; i < positions.count; i++) {
            const x = positions.getX(i);
            const y = positions.getY(i);
            const noise = Math.sin(x * 0.05) * Math.cos(y * 0.05) * 2 + 
                         Math.sin(x * 0.02 + 1) * Math.cos(y * 0.02) * 3;
            positions.setZ(i, noise);
        }
        groundGeometry.computeVertexNormals();

        const groundMaterial = new THREE.MeshStandardMaterial({
            color: 0x8d6e63,
            roughness: 0.9,
            metalness: 0.1,
            flatShading: true
        });

        this.groundMesh = new THREE.Mesh(groundGeometry, groundMaterial);
        this.groundMesh.rotation.x = -Math.PI / 2;
        this.groundMesh.receiveShadow = true;
        this.scene.add(this.groundMesh);

        const undergroundGeometry = new THREE.BoxGeometry(300, 100, 200);
        const undergroundMaterial = new THREE.MeshStandardMaterial({
            color: 0x6d4c41,
            roughness: 1.0,
            metalness: 0.0,
            transparent: true,
            opacity: 0.15,
            side: THREE.BackSide
        });
        const underground = new THREE.Mesh(undergroundGeometry, undergroundMaterial);
        underground.position.y = -50;
        this.scene.add(underground);
    }

    createDefaultKarez() {
        const defaultSegments = [
            { id: 1, name: '首部暗渠段', startX: -100, startY: 85, endX: -60, endY: 80, width: 0.8, height: 1.2, length: 800 },
            { id: 2, name: '中部暗渠段', startX: -60, startY: 80, endX: 30, endY: 70, width: 0.8, height: 1.2, length: 1800 },
            { id: 3, name: '尾部暗渠段', startX: 30, startY: 70, endX: 110, endY: 55, width: 0.8, height: 1.2, length: 1600 },
            { id: 4, name: '龙口段', startX: 110, startY: 55, endX: 160, endY: -5, width: 1.0, height: 1.5, length: 1000 },
        ];

        this.segments = defaultSegments;

        defaultSegments.forEach((seg, index) => {
            this.createAqueductSegment(seg, index);
            this.createWaterParticles(seg, index);
        });

        this.createShafts();
    }

    createAqueductSegment(segment, index) {
        const scaleFactor = 0.5;
        const startX = segment.startX * scaleFactor;
        const startY = -segment.startY * 0.3;
        const endX = segment.endX * scaleFactor;
        const endY = -segment.endY * 0.3;

        const dx = endX - startX;
        const dy = endY - startY;
        const length = Math.sqrt(dx * dx + dy * dy);
        const angle = Math.atan2(dy, dx);

        const pipeWidth = segment.width * 4;
        const pipeHeight = segment.height * 4;

        const shape = new THREE.Shape();
        const hw = pipeWidth / 2;
        const hh = pipeHeight / 2;
        shape.moveTo(-hw, -hh);
        shape.lineTo(-hw, hh * 0.5);
        shape.quadraticCurveTo(-hw, hh, -hw * 0.5, hh);
        shape.lineTo(hw * 0.5, hh);
        shape.quadraticCurveTo(hw, hh, hw, hh * 0.5);
        shape.lineTo(hw, -hh);
        shape.lineTo(-hw, -hh);

        const extrudeSettings = {
            steps: 50,
            depth: length,
            bevelEnabled: false
        };

        const geometry = new THREE.ExtrudeGeometry(shape, extrudeSettings);

        const material = new THREE.MeshPhysicalMaterial({
            color: 0x64b5f6,
            transparent: true,
            opacity: 0.25,
            roughness: 0.1,
            metalness: 0.1,
            transmission: 0.6,
            thickness: 0.5,
            side: THREE.DoubleSide
        });

        const pipe = new THREE.Mesh(geometry, material);
        pipe.rotation.y = -angle;
        pipe.position.set(startX, startY, 0);
        
        pipe.castShadow = false;
        pipe.receiveShadow = false;

        const edgeGeometry = new THREE.EdgesGeometry(geometry);
        const edgeMaterial = new THREE.LineBasicMaterial({
            color: 0x4a90d9,
            transparent: true,
            opacity: 0.6
        });
        const edges = new THREE.LineSegments(edgeGeometry, edgeMaterial);
        pipe.add(edges);

        pipe.userData = { 
            segmentId: segment.id, 
            segmentName: segment.name,
            type: 'aqueduct'
        };

        this.scene.add(pipe);
        this.aqueductMeshes.push(pipe);
    }

    createWaterParticles(segment, index) {
        const particleCount = 300;
        const scaleFactor = 0.5;
        const startX = segment.startX * scaleFactor;
        const startY = -segment.startY * 0.3;
        const endX = segment.endX * scaleFactor;
        const endY = -segment.endY * 0.3;

        const dx = endX - startX;
        const dy = endY - startY;
        const length = Math.sqrt(dx * dx + dy * dy);
        const angle = Math.atan2(dy, dx);

        const pipeWidth = segment.width * 4;
        const pipeHeight = segment.height * 4;

        const geometry = new THREE.BufferGeometry();
        const positions = new Float32Array(particleCount * 3);
        const speeds = new Float32Array(particleCount);
        const sizes = new Float32Array(particleCount);

        for (let i = 0; i < particleCount; i++) {
            const t = Math.random();
            const localX = (Math.random() - 0.5) * pipeWidth * 0.7;
            const localY = (Math.random() - 0.5) * pipeHeight * 0.6;
            const localZ = t * length;

            positions[i * 3] = localX;
            positions[i * 3 + 1] = localY;
            positions[i * 3 + 2] = localZ;

            speeds[i] = 0.5 + Math.random() * 0.5;
            sizes[i] = 0.3 + Math.random() * 0.4;
        }

        geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
        geometry.setAttribute('size', new THREE.BufferAttribute(sizes, 1));

        const material = new THREE.PointsMaterial({
            color: 0x4fc3f7,
            size: 0.3,
            transparent: true,
            opacity: 0.8,
            blending: THREE.AdditiveBlending,
            sizeAttenuation: true
        });

        const particles = new THREE.Points(geometry, material);
        particles.rotation.y = -angle;
        particles.position.set(startX, startY, 0);
        
        particles.userData = {
            speeds: speeds,
            length: length,
            pipeWidth: pipeWidth,
            pipeHeight: pipeHeight,
            segmentId: segment.id,
            flowSpeed: 1.0
        };

        this.scene.add(particles);
        this.particleSystems.push(particles);
    }

    createShafts() {
        const shaftCount = 15;
        const scaleFactor = 0.5;

        for (let i = 0; i < shaftCount; i++) {
            const t = i / (shaftCount - 1);
            
            let segIndex = 0;
            let segT = t;
            let cumLength = 0;
            const totalLength = this.segments.reduce((sum, s) => sum + (s.endX - s.startX), 0);
            const targetDist = t * totalLength;
            
            let accDist = 0;
            for (let j = 0; j < this.segments.length; j++) {
                const segLen = this.segments[j].endX - this.segments[j].startX;
                if (accDist + segLen >= targetDist) {
                    segIndex = j;
                    segT = (targetDist - accDist) / segLen;
                    break;
                }
                accDist += segLen;
            }

            const seg = this.segments[segIndex];
            const x = seg.startX + segT * (seg.endX - seg.startX);
            const y = seg.startY + segT * (seg.endY - seg.startY);

            const shaftDepth = 120 + i * 2;
            const groundY = 5 + Math.sin(i * 0.5) * 2;

            this.createShaft(
                x * scaleFactor,
                groundY,
                -y * 0.3,
                shaftDepth * 0.3,
                i
            );
        }
    }

    createShaft(x, groundY, bottomY, depth, index) {
        const shaftDiameter = 1.5;
        const shaftRadius = shaftDiameter / 2;

        const geometry = new THREE.CylinderGeometry(
            shaftRadius, 
            shaftRadius * 0.9, 
            depth, 
            12, 
            1,
            true
        );

        const material = new THREE.MeshStandardMaterial({
            color: 0x8d6e63,
            roughness: 0.9,
            metalness: 0.1,
            side: THREE.DoubleSide,
            transparent: true,
            opacity: 0.8
        });

        const shaft = new THREE.Mesh(geometry, material);
        shaft.position.set(x, groundY - depth / 2, 0);
        shaft.castShadow = true;
        shaft.receiveShadow = true;

        shaft.userData = {
            shaftId: index + 1,
            shaftName: `竖井-${index + 1}`,
            type: 'shaft',
            depth: depth
        };

        const topRingGeometry = new THREE.TorusGeometry(shaftRadius + 0.1, 0.15, 8, 16);
        const topRingMaterial = new THREE.MeshStandardMaterial({
            color: 0x5d4037,
            roughness: 0.8,
            metalness: 0.2
        });
        const topRing = new THREE.Mesh(topRingGeometry, topRingMaterial);
        topRing.rotation.x = Math.PI / 2;
        topRing.position.y = depth / 2;
        shaft.add(topRing);

        this.scene.add(shaft);
        this.shaftMeshes.push(shaft);
    }

    updateParticles(delta) {
        if (!this.showParticles) return;

        this.particleSystems.forEach(system => {
            const positions = system.geometry.attributes.position.array;
            const speeds = system.userData.speeds;
            const length = system.userData.length;
            const pipeWidth = system.userData.pipeWidth;
            const pipeHeight = system.userData.pipeHeight;
            const flowSpeed = system.userData.flowSpeed || 1.0;

            for (let i = 0; i < speeds.length; i++) {
                let z = positions[i * 3 + 2];
                z += speeds[i] * flowSpeed * this.simSpeed * delta * 10;

                if (z > length) {
                    z = 0;
                    positions[i * 3] = (Math.random() - 0.5) * pipeWidth * 0.7;
                    positions[i * 3 + 1] = (Math.random() - 0.5) * pipeHeight * 0.6;
                }

                positions[i * 3 + 2] = z;
            }

            system.geometry.attributes.position.needsUpdate = true;
        });
    }

    updateFlowData(flowData) {
        this.flowData = flowData;
        
        this.particleSystems.forEach(system => {
            const segId = system.userData.segmentId;
            if (flowData[segId] !== undefined) {
                system.userData.flowSpeed = Math.max(0.1, flowData[segId] * 15);
            }
        });
    }

    setView(view) {
        const duration = 1000;
        const startPos = this.camera.position.clone();
        const startTarget = this.controls.target.clone();

        let endPos, endTarget;

        switch (view) {
            case 'front':
                endPos = new THREE.Vector3(0, 30, 100);
                endTarget = new THREE.Vector3(0, -20, 0);
                break;
            case 'side':
                endPos = new THREE.Vector3(100, 30, 0);
                endTarget = new THREE.Vector3(0, -20, 0);
                break;
            case 'top':
                endPos = new THREE.Vector3(0, 100, 0.1);
                endTarget = new THREE.Vector3(0, 0, 0);
                break;
            case '3d':
            default:
                endPos = new THREE.Vector3(60, 40, 80);
                endTarget = new THREE.Vector3(0, -20, 0);
                break;
        }

        this.animateCamera(startPos, endPos, startTarget, endTarget, duration);
    }

    animateCamera(startPos, endPos, startTarget, endTarget, duration) {
        const startTime = performance.now();

        const animate = () => {
            const elapsed = performance.now() - startTime;
            const t = Math.min(elapsed / duration, 1);
            const eased = t < 0.5 ? 2 * t * t : -1 + (4 - 2 * t) * t;

            this.camera.position.lerpVectors(startPos, endPos, eased);
            this.controls.target.lerpVectors(startTarget, endTarget, eased);
            this.controls.update();

            if (t < 1) {
                requestAnimationFrame(animate);
            }
        };

        animate();
    }

    resetCamera() {
        this.setView('3d');
    }

    toggleParticles(show) {
        this.showParticles = show;
        this.particleSystems.forEach(p => p.visible = show);
    }

    toggleShafts(show) {
        this.showShafts = show;
        this.shaftMeshes.forEach(s => s.visible = show);
    }

    setSimSpeed(speed) {
        this.simSpeed = speed;
    }

    highlightSegment(segmentId) {
        this.aqueductMeshes.forEach(mesh => {
            if (mesh.userData.segmentId === segmentId) {
                mesh.material.opacity = 0.5;
                mesh.material.emissive = new THREE.Color(0x4fc3f7);
                mesh.material.emissiveIntensity = 0.3;
            } else {
                mesh.material.opacity = 0.25;
                mesh.material.emissive = new THREE.Color(0x000000);
                mesh.material.emissiveIntensity = 0;
            }
        });
    }

    onResize() {
        const width = this.container.clientWidth;
        const height = this.container.clientHeight;

        this.camera.aspect = width / height;
        this.camera.updateProjectionMatrix();

        this.renderer.setSize(width, height);
    }

    animate() {
        requestAnimationFrame(() => this.animate());

        const delta = this.clock.getDelta();

        this.updateParticles(delta);
        this.controls.update();

        this.renderer.render(this.scene, this.camera);
    }

    loadFromData(segments, shafts) {
        this.aqueductMeshes.forEach(m => this.scene.remove(m));
        this.particleSystems.forEach(p => this.scene.remove(p));
        this.shaftMeshes.forEach(s => this.scene.remove(s));
        
        this.aqueductMeshes = [];
        this.particleSystems = [];
        this.shaftMeshes = [];

        this.segments = segments;

        segments.forEach((seg, index) => {
            const segData = {
                id: seg.id,
                name: seg.segment_name,
                startX: -80 + index * 50,
                startY: 90 - index * 10,
                endX: -80 + (index + 1) * 50,
                endY: 80 - index * 10,
                width: seg.width,
                height: seg.height,
                length: seg.length
            };
            this.createAqueductSegment(segData, index);
            this.createWaterParticles(segData, index);
        });

        if (shafts && shafts.length > 0) {
            shafts.forEach((shaft, i) => {
                const x = -70 + (i / shafts.length) * 140;
                const groundY = 5;
                const bottomY = -shaft.shaft_depth * 0.3;
                this.createShaft(x, groundY, bottomY, shaft.shaft_depth * 0.3, i);
            });
        } else {
            this.createShafts();
        }
    }
}

let karezViewer = null;

function initKarez3D() {
    karezViewer = new Karez3DViewer('canvas-container');
}

function setView(view) {
    if (karezViewer) {
        karezViewer.setView(view);
    }
}

function resetCamera() {
    if (karezViewer) {
        karezViewer.resetCamera();
    }
}

function toggleParticles(show) {
    if (karezViewer) {
        karezViewer.toggleParticles(show);
    }
}

function toggleShafts(show) {
    if (karezViewer) {
        karezViewer.toggleShafts(show);
    }
}

function updateSimSpeed(value) {
    if (karezViewer) {
        karezViewer.setSimSpeed(parseFloat(value));
    }
    document.getElementById('simSpeedValue').textContent = value + 'x';
}
